package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/aqutw/travel-golang-api/internal/workspace"
	"github.com/gin-gonic/gin"
)

const MaxBodyBytes int64 = 64 << 10

type Options struct {
	Store          *workspace.Store
	APIKey         string
	AllowedOrigins []string
	Logger         *slog.Logger
	RatePerMinute  int
	Burst          int
}

func defaultOptions() Options { return Options{}.withDefaults() }
func (o Options) withDefaults() Options {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if len(o.AllowedOrigins) == 0 {
		o.AllowedOrigins = []string{"*"}
	}
	if o.RatePerMinute <= 0 {
		o.RatePerMinute = 60
	}
	if o.Burst <= 0 {
		o.Burst = 20
	}
	return o
}
func abort(c *gin.Context, status int, code, message string) {
	c.Abort()
	failure(c.Writer, status, code, message)
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if !validRequestID.MatchString(id) {
			id = rand.Text()
		}
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		c.Next()
	}
}
func accessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		// Route patterns exclude customer input, query strings, bodies and credentials.
		logger.Info("http_request", "request_id", c.GetString("request_id"), "method", c.Request.Method, "route", route, "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}
func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				logger.Error("request panic", "request_id", c.GetString("request_id"))
				if !c.Writer.Written() {
					abort(c, 500, "internal_error", "internal server error")
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}

type headWriter struct{ gin.ResponseWriter }

func (w headWriter) Write(b []byte) (int, error)       { w.WriteHeaderNow(); return len(b), nil }
func (w headWriter) WriteString(b string) (int, error) { w.WriteHeaderNow(); return len(b), nil }
func transport(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodHead {
			c.Writer = headWriter{c.Writer}
		}
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Idempotency-Key, X-Request-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID, Location, Idempotency-Replayed, Retry-After")
		origin := c.GetHeader("Origin")
		allowed := false
		for _, o := range opts.AllowedOrigins {
			if o == "*" {
				allowed = true
				c.Header("Access-Control-Allow-Origin", "*")
				break
			}
			if o == origin {
				allowed = true
				c.Header("Access-Control-Allow-Origin", origin)
			}
		}
		if !(len(opts.AllowedOrigins) == 1 && opts.AllowedOrigins[0] == "*") {
			c.Writer.Header().Add("Vary", "Origin")
		}
		if origin != "" && !allowed {
			abort(c, 403, "origin_not_allowed", "origin is not allowed")
			return
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
func bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "POST" || c.Request.Method == "PUT" {
			if c.Request.ContentLength > MaxBodyBytes {
				abort(c, 413, "body_too_large", "JSON body must be at most 65536 bytes")
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)
		}
		c.Next()
	}
}
func authorize(key string) gin.HandlerFunc {
	expected := sha256.Sum256([]byte(key))
	return func(c *gin.Context) {
		if key == "" {
			abort(c, 503, "writes_disabled", "configure API_WRITE_KEY to enable workspace access")
			return
		}
		supplied := sha256.Sum256([]byte(c.GetHeader("X-API-Key")))
		if subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 {
			abort(c, 401, "unauthorized", "a valid X-API-Key header is required")
			return
		}
		c.Next()
	}
}

type bucket struct {
	tokens  float64
	updated time.Time
}

func newLimiter(opts Options) gin.HandlerFunc {
	var mu sync.Mutex
	buckets := map[string]bucket{}
	return func(c *gin.Context) {
		if c.Request.Method != "POST" && c.Request.Method != "PUT" {
			c.Next()
			return
		}
		now := time.Now()
		ip := c.ClientIP()
		mu.Lock()
		b, exists := buckets[ip]
		if !exists {
			for k, v := range buckets {
				if now.Sub(v.updated) > 10*time.Minute {
					delete(buckets, k)
				}
			}
			if len(buckets) >= 1024 {
				mu.Unlock()
				c.Header("Retry-After", "60")
				abort(c, 429, "rate_limited", "write request limit exceeded")
				return
			}
			b = bucket{float64(opts.Burst), now}
		}
		b.tokens = math.Min(float64(opts.Burst), b.tokens+now.Sub(b.updated).Seconds()*float64(opts.RatePerMinute)/60)
		b.updated = now
		permitted := b.tokens >= 1
		retry := int(math.Ceil((1 - b.tokens) * 60 / float64(opts.RatePerMinute)))
		if permitted {
			b.tokens--
		}
		buckets[ip] = b
		mu.Unlock()
		if !permitted {
			c.Header("Retry-After", fmtInt(max(1, retry)))
			abort(c, 429, "rate_limited", "write request limit exceeded")
			return
		}
		c.Next()
	}
}

func fmtInt(n int) string { return strconv.Itoa(n) }

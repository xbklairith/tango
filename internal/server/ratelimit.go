package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig holds configuration for the per-IP rate limiter.
type RateLimitConfig struct {
	// GeneralRPS is the requests-per-second limit for general endpoints.
	GeneralRPS int
	// GeneralBurst is the burst capacity for general endpoints.
	GeneralBurst int
	// AuthRPS is the requests-per-second limit for auth endpoints.
	AuthRPS int
	// AuthBurst is the burst capacity for auth endpoints.
	AuthBurst int
	// CleanupAge is how long an IP entry must be idle before eviction.
	CleanupAge time.Duration
	// TrustedProxies is a comma-separated list of CIDR ranges.
	TrustedProxies string
}

type ipLimiter struct {
	general  *rate.Limiter
	auth     *rate.Limiter
	lastSeen time.Time
}

// RateLimitMiddleware provides per-IP token-bucket rate limiting.
type RateLimitMiddleware struct {
	mu             sync.Mutex
	limiters       map[string]*ipLimiter
	config         RateLimitConfig
	trustedProxies []*net.IPNet
}

// NewRateLimitMiddleware creates a new rate limiter middleware.
func NewRateLimitMiddleware(cfg RateLimitConfig) *RateLimitMiddleware {
	proxies, _ := parseTrustedProxies(cfg.TrustedProxies)
	return &RateLimitMiddleware{
		limiters:       make(map[string]*ipLimiter),
		config:         cfg,
		trustedProxies: proxies,
	}
}

// Middleware returns the HTTP middleware function.
func (rl *RateLimitMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r, rl.trustedProxies)
			limiter := rl.getLimiter(ip)

			var lim *rate.Limiter
			if isAuthEndpoint(r.URL.Path) {
				lim = limiter.auth
			} else {
				lim = limiter.general
			}

			if !lim.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error: "Rate limit exceeded",
					Code:  "RATE_LIMITED",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// StartCleanup launches a goroutine that periodically evicts stale IP entries.
func (rl *RateLimitMiddleware) StartCleanup(ctx context.Context) {
	go func() {
		cleanupAge := rl.config.CleanupAge
		if cleanupAge == 0 {
			cleanupAge = 5 * time.Minute
		}
		ticker := time.NewTicker(cleanupAge)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rl.cleanup(cleanupAge)
			}
		}
	}()
}

func (rl *RateLimitMiddleware) cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for ip, l := range rl.limiters {
		if l.lastSeen.Before(cutoff) {
			delete(rl.limiters, ip)
		}
	}
}

func (rl *RateLimitMiddleware) getLimiter(ip string) *ipLimiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	l, exists := rl.limiters[ip]
	if !exists {
		l = &ipLimiter{
			general:  rate.NewLimiter(rate.Limit(rl.config.GeneralRPS), rl.config.GeneralBurst),
			auth:     rate.NewLimiter(rate.Limit(rl.config.AuthRPS), rl.config.AuthBurst),
			lastSeen: time.Now(),
		}
		rl.limiters[ip] = l
	}
	l.lastSeen = time.Now()
	return l
}

// extractIP returns the client IP, respecting trusted proxies for X-Forwarded-For.
func extractIP(r *http.Request, trustedProxies []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	remoteIP := net.ParseIP(host)
	if remoteIP != nil && len(trustedProxies) > 0 {
		isTrusted := false
		for _, cidr := range trustedProxies {
			if cidr.Contains(remoteIP) {
				isTrusted = true
				break
			}
		}
		if isTrusted {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				// Walk from right to left, find first non-trusted IP
				parts := strings.Split(xff, ",")
				for i := len(parts) - 1; i >= 0; i-- {
					ip := strings.TrimSpace(parts[i])
					if ip == "" {
						continue
					}
					parsed := net.ParseIP(ip)
					if parsed == nil {
						// Unparseable entry — treat as untrusted client
						return ip
					}
					trusted := false
					for _, cidr := range trustedProxies {
						if cidr.Contains(parsed) {
							trusted = true
							break
						}
					}
					if !trusted {
						return ip
					}
				}
			}
		}
	}

	return host
}

// parseTrustedProxies parses a comma-separated list of CIDR strings.
func parseTrustedProxies(cidrs string) ([]*net.IPNet, error) {
	if cidrs == "" {
		return nil, nil
	}
	var nets []*net.IPNet
	for _, cidr := range strings.Split(cidrs, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		nets = append(nets, ipNet)
	}
	return nets, nil
}

// authPaths lists paths that should use stricter auth-tier limits.
var authPaths = map[string]bool{
	"/api/auth/login":    true,
	"/api/auth/register": true,
}

// isAuthEndpoint returns true if the path should use auth-tier rate limits.
func isAuthEndpoint(path string) bool {
	if authPaths[path] {
		return true
	}
	return strings.Contains(path, "/api/auth/oauth/") && strings.HasSuffix(path, "/callback")
}

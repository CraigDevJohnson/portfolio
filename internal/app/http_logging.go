package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"portfolio/internal/httpx"
	"portfolio/internal/logging"
)

type loggingResponseWriter struct {
	http.ResponseWriter

	bytesWritten int
	statusCode   int
}

func (writer *loggingResponseWriter) WriteHeader(statusCode int) {
	writer.statusCode = statusCode
	writer.ResponseWriter.WriteHeader(statusCode)
}

func (writer *loggingResponseWriter) Write(body []byte) (int, error) {
	if writer.statusCode == 0 {
		writer.statusCode = http.StatusOK
	}
	bytesWritten, err := writer.ResponseWriter.Write(body)
	writer.bytesWritten += bytesWritten
	return bytesWritten, err
}

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = logging.Component("http")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = logging.NewRequestID()
		}

		request := r.WithContext(logging.WithRequestID(r.Context(), requestID))
		w.Header().Set("X-Request-ID", requestID)

		recorder := &loggingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		startedAt := time.Now()

		next.ServeHTTP(recorder, request)

		route := request.Pattern
		if strings.TrimSpace(route) == "" {
			route = "unmatched"
		}

		duration := time.Since(startedAt)
		attrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", request.Method),
			slog.String("path", request.URL.Path),
			slog.String("route", route),
			slog.Int("status", recorder.statusCode),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.Int("bytes_written", recorder.bytesWritten),
			slog.String("client_ip", httpx.ClientIP(request)),
		}
		if userAgent := strings.TrimSpace(request.UserAgent()); userAgent != "" {
			attrs = append(attrs, slog.String("user_agent", userAgent))
		}

		level := requestLogLevel(request, recorder.statusCode)

		logger.LogAttrs(request.Context(), level, "request completed", attrs...)
	})
}

func requestLogLevel(request *http.Request, statusCode int) slog.Level {
	switch {
	case statusCode >= http.StatusInternalServerError:
		return slog.LevelError
	case statusCode >= http.StatusBadRequest:
		if statusCode == http.StatusNotFound && request.URL.Path == "/.well-known/appspecific/com.chrome.devtools.json" {
			return slog.LevelInfo
		}
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

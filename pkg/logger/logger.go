package logger

import (
"fmt"
"os"
"time"

"github.com/gin-gonic/gin"
"github.com/rs/zerolog"
"github.com/rs/zerolog/log"
)

var logger zerolog.Logger

func Init() {
zerolog.TimeFieldFormat = time.RFC3339
output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
logger = zerolog.New(output).With().Timestamp().Caller().Logger()
log.Logger = logger
}

func Debug(format string, v ...interface{}) {
logger.Debug().Msg(fmt.Sprintf(format, v...))
}

func Info(format string, v ...interface{}) {
logger.Info().Msg(fmt.Sprintf(format, v...))
}

func Warn(format string, v ...interface{}) {
logger.Warn().Msg(fmt.Sprintf(format, v...))
}

func Error(format string, v ...interface{}) {
logger.Error().Msg(fmt.Sprintf(format, v...))
}

func Fatal(format string, v ...interface{}) {
logger.Fatal().Msg(fmt.Sprintf(format, v...))
}

func GinLogger() gin.HandlerFunc {
return func(c *gin.Context) {
start := time.Now()
path := c.Request.URL.Path
raw := c.Request.URL.RawQuery

c.Next()

latency := time.Since(start)
clientIP := c.ClientIP()
method := c.Request.Method
statusCode := c.Writer.Status()
errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

if raw != "" {
path = path + "?" + raw
}

logEvent := logger.Info()
if statusCode >= 400 {
logEvent = logger.Error()
}

logEvent.
Str("client_ip", clientIP).
Str("method", method).
Str("path", path).
Int("status", statusCode).
Dur("latency", latency).
Str("error", errorMessage).
Msg("HTTP Request")
}
}

package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// captureProcessOutput redirects os.Stdout, os.Stderr and gin's default writers
// while fn runs and returns everything written to them.
func captureProcessOutput(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	previousStdout, previousStderr := os.Stdout, os.Stderr
	previousGinOut, previousGinErr := gin.DefaultWriter, gin.DefaultErrorWriter
	os.Stdout, os.Stderr = writer, writer
	gin.DefaultWriter, gin.DefaultErrorWriter = writer, writer

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		done <- buf.String()
	}()

	func() {
		defer func() {
			os.Stdout, os.Stderr = previousStdout, previousStderr
			gin.DefaultWriter, gin.DefaultErrorWriter = previousGinOut, previousGinErr
		}()
		fn()
	}()

	require.NoError(t, writer.Close())
	output := <-done
	require.NoError(t, reader.Close())
	return output
}

func newPanicRouter(mode string) *gin.Engine {
	gin.SetMode(mode)
	router := gin.New()
	router.Use(Recovery(), RequestID())
	router.GET("/v1/citizen/:cpf/wallet", func(c *gin.Context) {
		panic(fmt.Sprintf("wallet failed for cpf=%s email %s", c.Param("cpf"), syntheticEmail))
	})
	router.GET("/v1/memory/:phone_number", func(c *gin.Context) {
		panic(fmt.Errorf("memory lookup %s: %w", c.Param("phone_number"), io.ErrUnexpectedEOF))
	})
	return router
}

func TestRecovery_PanicLogsMaskedRequestWithoutRawDump(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantRoute string
		wantPath  string
		wantType  string
	}{
		{
			name:      "CPF route, string panic",
			target:    "/v1/citizen/" + syntheticCPF + "/wallet?cpf=" + syntheticCPF,
			wantRoute: "/v1/citizen/:cpf/wallet",
			wantPath:  "/v1/citizen/***/wallet",
			wantType:  "string",
		},
		{
			name:      "phone route, error panic",
			target:    "/v1/memory/" + syntheticPhone,
			wantRoute: "/v1/memory/:phone_number",
			wantPath:  "/v1/memory/***",
			wantType:  "*fmt.wrapError",
		},
	}
	// Debug mode is where gin.Recovery() also dumps the request headers; it must not matter here.
	for _, mode := range []string{gin.ReleaseMode, gin.DebugMode} {
		for _, tt := range tests {
			t.Run(mode+"/"+tt.name, func(t *testing.T) {
				logs := observeGlobalLogger(t)
				recorder := httptest.NewRecorder()

				// The router is built inside the capture: gin.Recovery() binds gin.DefaultErrorWriter
				// when it is created, so a raw dump would land in the captured output.
				output := captureProcessOutput(t, func() {
					router := newPanicRouter(mode)
					req := httptest.NewRequest(http.MethodGet, tt.target, nil)
					req.Header.Set("Authorization", "Bearer token-sintetico")
					req.Header.Set("X-Citizen-Cpf", syntheticCPF)
					router.ServeHTTP(recorder, req)
				})

				assert.Equal(t, http.StatusInternalServerError, recorder.Code)
				// Checked before the log entry, so a raw dump fails the test on its own.
				for _, raw := range syntheticPersonalData {
					assert.NotContains(t, output, raw, "personal data written to stdout/stderr")
				}
				assert.NotContains(t, output, "HTTP/1.1", "raw request dumped to stdout/stderr")
				assert.NotContains(t, output, "panic recovered")

				entries := logs.FilterMessage("panic recovered").All()
				require.Len(t, entries, 1)
				assert.Equal(t, zapcore.ErrorLevel, entries[0].Level)
				fields := entries[0].ContextMap()
				assert.Equal(t, tt.wantRoute, fields["route"])
				assert.Equal(t, tt.wantPath, fields["path"])
				assert.Equal(t, http.MethodGet, fields["method"])
				assert.Equal(t, tt.wantType, fields["panic_type"])
				assert.Contains(t, fields["panic"], "***")
				assert.NotEmpty(t, fields["request_id"])
				assertNoPersonalDataInLogs(t, logs)
			})
		}
	}
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	logs := observeGlobalLogger(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/v1/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, logs.FilterMessage("panic recovered").All())
}

func TestRecovery_PanicAfterResponseWrittenKeepsStatus(t *testing.T) {
	logs := observeGlobalLogger(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/v1/citizen/:cpf", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
		panic("after write")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/citizen/"+syntheticCPF, nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, logs.FilterMessage("panic recovered").All(), 1)
	assertNoPersonalDataInLogs(t, logs)
}

func TestRecovery_BrokenConnectionIsWarningWithoutStatus(t *testing.T) {
	logs := observeGlobalLogger(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/v1/memory/:phone_number", func(c *gin.Context) {
		panic(&net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.EPIPE)})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/memory/"+syntheticPhone, nil))

	assert.False(t, recorder.Flushed)
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, logs.FilterMessage("panic recovered").All())
	warnings := logs.FilterMessage("client connection closed during request").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, zapcore.WarnLevel, warnings[0].Level)
	assert.Equal(t, "/v1/memory/***", warnings[0].ContextMap()["path"])
	assertNoPersonalDataInLogs(t, logs)
}

func TestMaskPanicValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "runtime-like message", value: "index out of range [5] with length 3", want: "index out of range [5] with length 3"},
		{name: "CPF digits", value: "no citizen " + syntheticCPF, want: "no citizen ***"},
		{name: "formatted CPF after key", value: "cpf:123.456.789-01 missing", want: "cpf:*** missing"},
		{name: "phone with DDD", value: fmt.Errorf("send to (21) 98765-4321 failed"), want: "send to *** failed"},
		{name: "phone split by a space", value: "phone 98765 4321 invalid", want: "phone *** invalid"},
		{name: "phone with DDI after colon", value: "lookup " + syntheticPhone + ": EOF", want: "lookup ***: EOF"},
		{name: "short numbers kept", value: "slice [3:7] of 12 items, retry 2 of 5", want: "slice [3:7] of 12 items, retry 2 of 5"},
		{name: "e-mail", value: "bad " + syntheticEmail, want: "bad ***"},
		{name: "path with CPF", value: "GET /v1/citizen/" + syntheticCPF + "/wallet", want: "GET /v1/citizen/***/wallet"},
		{name: "non-string value", value: 42, want: "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, maskPanicValue(tt.value))
		})
	}
}

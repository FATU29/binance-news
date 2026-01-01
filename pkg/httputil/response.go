package httputil

import (
"github.com/gin-gonic/gin"
)

type Response struct {
	Success bool        `json:"success"`
	Status  string      `json:"status,omitempty"` // "success" or "error" for compatibility
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

type Error struct {
Code    string `json:"code"`
Message string `json:"message"`
Details string `json:"details,omitempty"`
}

func SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Success: true,
		Status:  "success",
		Data:    data,
	})
}

func ErrorResponse(c *gin.Context, statusCode int, message string, err error) {
	var details string
	if err != nil {
		details = err.Error()
	}

	c.JSON(statusCode, Response{
		Success: false,
		Status:  "error",
		Message: message,
		Error: &Error{
			Code:    getErrorCode(statusCode),
			Message: message,
			Details: details,
		},
	})
}

func getErrorCode(statusCode int) string {
switch statusCode {
case 400:
return "BAD_REQUEST"
case 401:
return "UNAUTHORIZED"
case 403:
return "FORBIDDEN"
case 404:
return "NOT_FOUND"
case 500:
return "INTERNAL_SERVER_ERROR"
default:
return "UNKNOWN_ERROR"
}
}

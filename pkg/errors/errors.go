package errors

// Keep all errors standardized between layers
type AppError struct {
	statusCode int
	message    string
}

// Use to initalize a new app error
func NewAppError(statusCode int, msg string) *AppError {
	return &AppError{
		statusCode: statusCode,
		message:    msg,
	}
}

// Get errror as string
func (app *AppError) Error() string {
	return app.message
}

// Get status code as int
func (app *AppError) StatusCode() int {
	return app.statusCode
}

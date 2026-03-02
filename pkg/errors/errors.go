package errors

type AppError struct {
	statusCode int
	message    string
}

func NewAppError(statusCode int, msg string) *AppError {
	return &AppError{
		statusCode: statusCode,
		message:    msg,
	}
}

func (app *AppError) Error() string {
	return app.message
}

func (app *AppError) StatusCode() int {
	return app.statusCode
}

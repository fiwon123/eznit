package sessions

import (
	"log/slog"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
)

type Service struct {
	db     Repository
	logger *logger.Config
}

func NewService(db Repository, logger *logger.Config) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

func (s *Service) IsValid(token string) bool {
	s.logger.Debug("token: ", slog.String("token", token))
	session := s.db.GetSession(token)
	if session == nil {
		s.logger.Warn("session not found")
		return false
	}

	s.logger.Debug("session: ", slog.Any("session", session))
	if !session.IsActive || session.ExpiresAt.Before(time.Now()) {
		s.logger.Warn("session is expired")
		return false
	}

	return true
}

func (s *Service) GetToken(userID string) *DataResponse {
	s.logger.Debug("userID: ", slog.String("id", userID))
	session := s.db.GetSessionByUserID(userID)
	if session == nil {
		s.logger.Error("session not found")
		return &DataResponse{
			Token: "",
		}
	}

	return &DataResponse{
		Token: session.Token,
	}
}

func (s *Service) CreateToken(userdID string) (string, bool) {

	s.logger.Debug("generating token...")
	token, err := helper.GenerateToken(32)
	if err != nil {
		s.logger.Error("can't generate token")
		return "", false
	}

	s.logger.Debug("token: ", slog.String("token", token))
	ok := s.db.CreateSession(Session{
		Token:     token,
		UserID:    userdID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if !ok {
		s.logger.Error("error create session db")
		return "", false
	}

	s.logger.Debug("token storaged!")

	return token, true
}

func (s *Service) UseToken(token string) bool {
	s.logger.Debug("token: ", slog.String("token", token))
	ok := s.db.UpdateSession(Session{
		Token:    token,
		IsActive: false,
	})

	if !ok {
		s.logger.Error("error update session db")
		return false
	}

	s.logger.Debug("token has been used!")

	return true
}

func (s *Service) GetUserIDByToken(token string) (string, bool) {
	userID, ok := s.db.GetUserIDByToken(token)
	if !ok {
		s.logger.Error("user not found")
		return "", false
	}

	s.logger.Debug("user found!")

	return userID, true
}

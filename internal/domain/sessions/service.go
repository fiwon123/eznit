package sessions

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
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
	s.logger.Debug("token: ", slog.String("token_fp", helper.Fingerprint(token)))
	session, err := s.db.GetSession(token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.logger.Warn("session not found")
		} else {
			s.logger.Error("get session", slog.Any("error", err))
		}
		return false
	}

	s.logger.Debug("session: ", slog.Any("session", session))
	if !session.IsActive || session.ExpiresAt.Before(time.Now()) {
		s.logger.Warn("session is expired")
		return false
	}

	return true
}

func (s *Service) GetToken(ctx context.Context, userID uuid.UUID) *DataResponse {
	s.logger.Debug("userID: ", slog.String("id", userID.String()))
	session, err := s.db.GetSessionByUserID(userID)
	if err != nil {
		s.logger.Error("session not found", slog.Any("error", err))
		return &DataResponse{
			Token: "",
		}
	}

	return &DataResponse{
		Token: session.Token,
	}
}

// Create a new token for user
func (s *Service) CreateToken(userdID uuid.UUID) (string, bool) {

	s.logger.Debug("generating token...")
	token, err := helper.GenerateToken(32)
	if err != nil {
		s.logger.Error("can't generate token")
		return "", false
	}

	s.logger.Debug("token: ", slog.String("token_fp", helper.Fingerprint(token)))
	err = s.db.CreateSession(Session{
		Token:     token,
		UserID:    userdID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if err != nil {
		s.logger.Error("error create session db", slog.Any("error", err))
		return "", false
	}

	s.logger.Debug("token storaged!")

	return token, true
}

// Invalidate a token after user logout
// token is active and user not loggout, repository has a expiration date time to control it too
func (s *Service) UseToken(ctx context.Context, token string) bool {
	s.logger.Debug("token: ", slog.String("token_fp", helper.Fingerprint(token)))
	err := s.db.UpdateSession(Session{
		Token:    token,
		IsActive: false,
	})

	if err != nil {
		s.logger.Error("error update session db", slog.Any("error", err))
		return false
	}

	s.logger.Debug("token has been used!")

	return true
}

// Get UserId by token string
func (s *Service) GetUserIDByToken(token string) (uuid.UUID, bool) {
	userID, err := s.db.GetUserIDByToken(token)
	if err != nil {
		s.logger.Error("user not found", slog.Any("error", err))
		return uuid.UUID{}, false
	}

	s.logger.Debug("user found!")

	return userID, true
}

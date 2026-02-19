package sessions

import (
	"fmt"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
)

type Service struct {
	db Repository
}

func NewService(db Repository) *Service {
	return &Service{
		db: db,
	}
}

func (s *Service) IsValid(token string) bool {
	session := s.db.GetSession(token)
	if session == nil {
		return false
	}

	if !session.IsActive || session.ExpiresAt.Before(time.Now()) {
		return false
	}

	return true
}

func (s *Service) GetToken(userID string) *DataResponse {
	session := s.db.GetSessionByUserID(userID)
	if session == nil {
		return &DataResponse{
			Token: "",
		}
	}

	return &DataResponse{
		Token: session.Token,
	}
}

func (s *Service) CreateToken(userdID string) (string, bool) {

	token, err := helper.GenerateToken(32)
	if err != nil {
		fmt.Println("can't generate token")
		return "", false
	}

	ok := s.db.CreateSession(Session{
		Token:     token,
		UserID:    userdID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if !ok {
		fmt.Println("error create session db")
		return "", false
	}

	return token, true
}

func (s *Service) UseToken(token string) bool {

	ok := s.db.UpdateSession(Session{
		Token:    token,
		IsActive: false,
	})

	if !ok {
		fmt.Println("error update session db")
		return false
	}

	return true
}

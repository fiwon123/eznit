package users

type Service struct {
	db Repository
}

func NewService(db Repository) *Service {
	return &Service{
		db: db,
	}
}

func (s *Service) GetUsers() []UserDataResponse {

	usersModel := s.db.GetUsers()

	resp := []UserDataResponse{}
	for _, user := range usersModel {
		resp = append(resp, UserDataResponse{
			Email: user.Email,
		})
	}

	return resp
}

func (s *Service) GetUser(id string) (UserDataResponse, bool) {

	user := s.db.GetUser(id)
	if user == nil {
		return UserDataResponse{}, false
	}

	resp := UserDataResponse{
		Email: user.Email,
	}

	return resp, true
}

func (s *Service) CreateUser(req UserCreate) (UserMsgResponse, bool) {

	db := s.db

	if db.UserExists(req.Email) {
		return UserMsgResponse{
			Msg: "user already exists",
		}, false
	}

	if !db.CreateUser(User{
		Email:    req.Email,
		Password: req.Password,
	}) {
		return UserMsgResponse{
			Msg: "internal server error",
		}, false
	}

	return UserMsgResponse{
		Msg: "user created!",
	}, true
}

func (s *Service) DeleteUser(req UserDelete) (UserMsgResponse, bool) {

	db := s.db

	user := db.GetUser(req.Id)
	if user == nil {
		return UserMsgResponse{
			Msg: "user not exists",
		}, false
	}

	if !db.DeleteUser(*user) {
		return UserMsgResponse{
			Msg: "internal server error",
		}, false
	}

	return UserMsgResponse{
		Msg: "user deleted!",
	}, true
}

func (s *Service) UpdateUser(req UserUpdate) (UserMsgResponse, bool) {

	db := s.db

	user := db.GetUser(req.Id)
	if user == nil {
		return UserMsgResponse{
			Msg: "user not exists",
		}, false
	}

	user.Email = req.Email
	user.Password = req.Password

	if !db.UpdateUser(*user) {
		return UserMsgResponse{
			Msg: "internal server error",
		}, false
	}

	return UserMsgResponse{
		Msg: "user updated!",
	}, true
}

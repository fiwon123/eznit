package users

type Service struct {
	db Repository
}

func NewService(db Repository) *Service {
	return &Service{
		db: db,
	}
}

func (s *Service) GetUsers() []DataResponse {

	usersModel := s.db.GetUsers()

	resp := []DataResponse{}
	for _, user := range usersModel {
		resp = append(resp, DataResponse{
			Email: user.Email,
		})
	}

	return resp
}

func (s *Service) GetUser(id string) (DataResponse, bool) {

	user := s.db.GetUser(id)
	if user == nil {
		return DataResponse{}, false
	}

	resp := DataResponse{
		Email: user.Email,
	}

	return resp, true
}

func (s *Service) CreateUser(req CreateRequest) (MsgResponse, bool) {

	db := s.db

	if db.UserExists(req.Email) {
		return MsgResponse{
			Msg: "user already exists",
		}, false
	}

	if !db.CreateUser(User{
		Email:    req.Email,
		Password: req.Password,
	}) {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	return MsgResponse{
		Msg: "user created!",
	}, true
}

func (s *Service) DeleteUser(req DeleteRequest) (MsgResponse, bool) {

	db := s.db

	user := db.GetUser(req.Id)
	if user == nil {
		return MsgResponse{
			Msg: "user not exists",
		}, false
	}

	if !db.DeleteUser(*user) {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	return MsgResponse{
		Msg: "user deleted!",
	}, true
}

func (s *Service) UpdateUser(req UpdateRequest) (MsgResponse, bool) {

	db := s.db

	user := db.GetUser(req.Id)
	if user == nil {
		return MsgResponse{
			Msg: "user not exists",
		}, false
	}

	user.Email = req.Email
	user.Password = req.Password

	if !db.UpdateUser(*user) {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	return MsgResponse{
		Msg: "user updated!",
	}, true
}

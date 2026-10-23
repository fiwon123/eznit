package commands

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler is responsible to handle all incoming request from the commands
// resource.
type Handler struct {
	service *service
	guard   *middleware.Guard
	logger  *logger.Config
}

// Return a new commands Handler
func NewHandler(service *service, guard *middleware.Guard, logger *logger.Config) *Handler {
	return &Handler{
		service: service,
		guard:   guard,
		logger:  logger,
	}
}

// Commands routes
func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Route("/v1/commands", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(h.guard.AuthUser)

			r.Get("/", h.listCommandsHandler)
			r.Post("/", h.createCommandHandler)
			r.Get("/{id}", h.getCommandHandler)
			r.Put("/{id}", h.updateCommandHandler)
			r.Delete("/{id}", h.deleteCommandHandler)
		})
	})
}

func (h *Handler) listCommandsHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("listCommandsHandler")

	dataResp, appError := h.service.ListCommands(r.Context())
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) getCommandHandler(w http.ResponseWriter, r *http.Request) {
	id, appError := extractCommandID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("getCommandHandler ", slog.String("id", id.String()))

	dataResp, appError := h.service.GetCommand(r.Context(), id)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) createCommandHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("createCommandHandler")

	request, ok := decodeCommandRequest(w, r, h.logger)
	if !ok {
		return
	}

	dataResp, appError := h.service.CreateCommand(r.Context(), request)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusCreated, dataResp)
}

func (h *Handler) updateCommandHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("updateCommandHandler")

	id, appError := extractCommandID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	request, ok := decodeCommandRequest(w, r, h.logger)
	if !ok {
		return
	}

	dataResp, appError := h.service.UpdateCommand(r.Context(), id, request)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) deleteCommandHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("deleteCommandHandler")

	id, appError := extractCommandID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	message, appError := h.service.DeleteCommand(r.Context(), id)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}

// decodeCommandRequest reads a command body. The limit is the binary cap plus
// room for base64 and the JSON around it.
func decodeCommandRequest(w http.ResponseWriter, r *http.Request, log *logger.Config) (CommandRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	var request CommandRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Warn("failed to decode: ", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusBadRequest, "failed to decode request")
		return CommandRequest{}, false
	}

	return request, true
}

// extractCommandID reads the command id from the path and parses it.
func extractCommandID(r *http.Request) (uuid.UUID, *errors.AppError) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.UUID{}, errors.NewAppError(http.StatusBadRequest, "invalid command id")
	}

	return id, nil
}

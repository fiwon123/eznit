package files

import (
	"log/slog"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Files Repository implementation
type sqlRepository struct {
	db     *sqlx.DB
	logger *logger.Config
}

// Return a new files repository
func NewRepository(db *sqlx.DB, logger *logger.Config) *sqlRepository {
	return &sqlRepository{
		db:     db,
		logger: logger,
	}
}

// Get a list of files
func (r *sqlRepository) GetFiles() ([]File, bool) {
	r.logger.Debug("getting files")

	var files []File

	err := r.db.Select(&files, "SELECT * FROM files")
	if err != nil {
		r.logger.Error("get files query", slog.Any("error", err))
		return []File{}, false
	}

	r.logger.Debug("files has been found")

	return files, true
}

// Get a list of files data for user by user id
func (r *sqlRepository) GetFilesForUser(userID uuid.UUID) ([]File, bool) {
	r.logger.Debug("getting files for user: ", slog.String("userID", userID.String()))

	var files []File

	err := r.db.Select(&files, "SELECT * FROM files WHERE user_id=$1", userID)
	if err != nil {
		r.logger.Error("get files for user", slog.Any("error", err))
		return []File{}, false
	}

	r.logger.Debug("files has been found")

	return files, true
}

// Get file data using id file
func (r *sqlRepository) GetFile(id uuid.UUID) (*File, bool) {

	r.logger.Debug("getting file: ", slog.String("id", id.String()))

	var file File

	err := r.db.Get(&file, "SELECT * FROM files WHERE id=$1", id)
	if err != nil {
		r.logger.Error("get file", slog.Any("error", err))
		return nil, false
	}

	r.logger.Debug("file has been found")

	return &file, true
}

// Get file data for user using id file and user id
func (r *sqlRepository) GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, bool) {

	r.logger.Debug("getting file for user: ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	var file File

	query := `SELECT * FROM files
		      WHERE id=$1 AND user_id=$2`

	err := r.db.Get(&file, query, id, userID)
	if err != nil {
		r.logger.Error("get file for user", slog.Any("error", err))
		return nil, false
	}

	r.logger.Debug("file has been found")

	return &file, true
}

// Storage file using file model
func (r *sqlRepository) StorageFile(file File) bool {

	r.logger.Debug("storaging file: ", slog.Any("file", file))

	query := `INSERT INTO files (id, user_id, name, ext, path, content_type)
          VALUES (:id, :user_id, :name, :ext, :path, :content_type)`

	_, err := r.db.NamedExec(query, file)
	if err != nil {
		r.logger.Error("storage file table files", slog.Any("error", err))
		return false
	}

	r.logger.Debug("file storaged!")

	return true
}

func (r *sqlRepository) StorageFileHistory(file File) bool {

	r.logger.Debug("storaging file history: ", slog.Any("file", file))

	_, err := r.db.NamedExec("INSERT INTO files_history (file_id, path, version) VALUES (:id, :path, :version)", file)
	if err != nil {
		r.logger.Error("storage file table files_history ", slog.Any("error", err))
		return false
	}

	r.logger.Debug("file history storaged!")

	return true
}

func (r *sqlRepository) DeleteFile(id uuid.UUID) bool {

	r.logger.Debug("deleting file: ", slog.String("id", id.String()))

	_, err := r.db.Exec("DELETE FROM files WHERE id=$1", id)
	if err != nil {
		r.logger.Error("delete file", slog.Any("error", err))
		return false
	}

	r.logger.Debug("file deleted!")

	return true

}

func (r *sqlRepository) DeleteFileForUser(id uuid.UUID, userID uuid.UUID) bool {
	r.logger.Debug("deleting file: ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	_, err := r.db.Exec("DELETE FROM files WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		r.logger.Error("delete file for user", slog.Any("error", err))
		return false
	}

	r.logger.Debug("file deleted!")

	return true

}

func (r sqlRepository) DeleteFileHistoryForUser(id uuid.UUID, userID uuid.UUID) bool {
	r.logger.Debug("deleting file history: ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	_, err := r.db.Exec(`DELETE FROM files_history AS h
		    			 USING files AS f, users AS u
						 WHERE f.user_id = u.id AND
							   f.id = h.file_id AND
							   h.file_id=$1 AND
							   f.user_id=$2`, id, userID)
	if err != nil {
		r.logger.Error("db failed to delete file history for user", slog.Any("error", err))
		return false
	}

	r.logger.Debug("file history deleted!")

	return true
}

func (r *sqlRepository) UpdateFile(file File) bool {

	r.logger.Debug("updating file: ", slog.Any("file", file))

	exec := "UPDATE files SET name=:name, ext=:ext, path=:path, version=:version, updated_at=NOW() WHERE id=:id"
	_, err := r.db.NamedExec(exec, file)
	if err != nil {
		r.logger.Error("update file table files", slog.Any("error", err))
		return false
	}

	_, err = r.db.NamedExec("UPDATE files_history SET path=:path, version=:version WHERE file_id=:id", file)
	if err != nil {
		r.logger.Error("update file table files_history", slog.Any("error", err))
		return false
	}

	r.logger.Debug("file updated!")

	return true
}

func (r *sqlRepository) IsUserOwner(id uuid.UUID, userID uuid.UUID) bool {
	r.logger.Debug("IsUserOwner", slog.String("id", id.String()), slog.String("userID", userID.String()))

	var count int

	query := "SELECT count(*) FROM files WHERE id=$1 AND user_id=$2"
	err := r.db.Get(&count, query, id, userID)
	if err != nil {
		r.logger.Error("failed to verify owner file", slog.Any("error", err))
		return false
	}

	if count <= 0 {
		return false
	}

	return true
}

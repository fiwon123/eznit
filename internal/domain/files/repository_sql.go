package files

import (
	"database/sql"
	"errors"
	"log/slog"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// fileColumns is the column list every read of the files table names. It is
// named, not `*`, because a migration that adds a column would otherwise break
// every read at run time: sqlx cannot scan a column the struct does not hold,
// and the repository would report "not found" for a file that is there.
// files/schema_test.go checks it against the struct and the migrations.
const fileColumns = "id, user_id, name, ext, path, version, content_type, is_public, created_at, updated_at"

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

// notFoundOr maps sql.ErrNoRows to ErrNotFound and returns any other error as is.
func notFoundOr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Get a list of files data for user by user id. The read is filtered by the
// owner in the query: a file belongs to the user who uploaded it, so no
// caller lists another user's rows in the first place.
func (r *sqlRepository) GetFilesForUser(userID uuid.UUID) ([]File, error) {
	r.logger.Debug("getting files for user: ", slog.String("userID", userID.String()))

	var files []File

	if err := r.db.Select(&files, "SELECT "+fileColumns+" FROM files WHERE user_id=$1", userID); err != nil {
		r.logger.Error("get files for user", slog.Any("error", err))
		return []File{}, err
	}

	r.logger.Debug("files has been found")

	return files, nil
}

// Get file data using id file
func (r *sqlRepository) GetFile(id uuid.UUID) (*File, error) {
	r.logger.Debug("getting file: ", slog.String("id", id.String()))

	var file File

	if err := r.db.Get(&file, "SELECT "+fileColumns+" FROM files WHERE id=$1", id); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Error("get file", slog.Any("error", err))
		}
		return nil, notFoundOr(err)
	}

	r.logger.Debug("file has been found")

	return &file, nil
}

// Get file data for user using id file and user id
func (r *sqlRepository) GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, error) {
	r.logger.Debug("getting file for user: ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	var file File

	query := "SELECT " + fileColumns + ` FROM files
		      WHERE id=$1 AND user_id=$2`

	if err := r.db.Get(&file, query, id, userID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Error("get file for user", slog.Any("error", err))
		}
		return nil, notFoundOr(err)
	}

	r.logger.Debug("file has been found")

	return &file, nil
}

// GetSharedFile returns a file if it is public (any authenticated caller may
// download a shared file). It does not check ownership.
func (r *sqlRepository) GetSharedFile(id uuid.UUID) (*File, error) {
	r.logger.Debug("getting shared file: ", slog.String("id", id.String()))

	var file File

	query := "SELECT " + fileColumns + " FROM files WHERE id=$1 AND is_public=TRUE"

	if err := r.db.Get(&file, query, id); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Error("get shared file", slog.Any("error", err))
		}
		return nil, notFoundOr(err)
	}

	r.logger.Debug("shared file has been found")

	return &file, nil
}

// ListSharedFiles returns all public files with their owner's email for the
// catalogue.
func (r *sqlRepository) ListSharedFiles() ([]SharedFileWithOwner, error) {
	r.logger.Debug("listing shared files")

	var files []SharedFileWithOwner

	query := "SELECT f.id, f.user_id, f.name, f.ext, f.path, f.version, f.content_type, f.is_public, f.created_at, f.updated_at, u.email AS owner_email FROM files f JOIN users u ON u.id = f.user_id WHERE f.is_public = TRUE ORDER BY f.updated_at DESC"

	if err := r.db.Select(&files, query); err != nil {
		r.logger.Error("list shared files", slog.Any("error", err))
		return []SharedFileWithOwner{}, err
	}

	return files, nil
}

// Storage file using file model
func (r *sqlRepository) StorageFile(file File) error {
	r.logger.Debug("storaging file: ", slog.Any("file", file))

	query := `INSERT INTO files (id, user_id, name, ext, path, content_type)
          VALUES (:id, :user_id, :name, :ext, :path, :content_type)`

	if _, err := r.db.NamedExec(query, file); err != nil {
		r.logger.Error("storage file table files", slog.Any("error", err))
		return err
	}

	r.logger.Debug("file storaged!")

	return nil
}

func (r *sqlRepository) StorageFileHistory(file File) error {
	r.logger.Debug("storaging file history: ", slog.Any("file", file))

	if _, err := r.db.NamedExec("INSERT INTO files_history (file_id, path, version) VALUES (:id, :path, :version)", file); err != nil {
		r.logger.Error("storage file table files_history ", slog.Any("error", err))
		return err
	}

	r.logger.Debug("file history storaged!")

	return nil
}

func (r *sqlRepository) DeleteFile(id uuid.UUID) error {
	r.logger.Debug("deleting file: ", slog.String("id", id.String()))

	result, err := r.db.Exec("DELETE FROM files WHERE id=$1", id)
	if err != nil {
		r.logger.Error("delete file", slog.Any("error", err))
		return err
	}

	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}

	r.logger.Debug("file deleted!")

	return nil
}

func (r *sqlRepository) DeleteFileForUser(id uuid.UUID, userID uuid.UUID) error {
	r.logger.Debug("deleting file: ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	result, err := r.db.Exec("DELETE FROM files WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		r.logger.Error("delete file for user", slog.Any("error", err))
		return err
	}

	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}

	r.logger.Debug("file deleted!")

	return nil
}

func (r *sqlRepository) DeleteFileHistoryForUser(id uuid.UUID, userID uuid.UUID) error {
	r.logger.Debug("deleting file history: ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	_, err := r.db.Exec(`DELETE FROM files_history AS h
		    			 USING files AS f, users AS u
						 WHERE f.user_id = u.id AND
							   f.id = h.file_id AND
							   h.file_id=$1 AND
							   f.user_id=$2`, id, userID)
	if err != nil {
		r.logger.Error("db failed to delete file history for user", slog.Any("error", err))
		return err
	}

	r.logger.Debug("file history deleted!")

	return nil
}

// UpdateFile moves the index row to the new version. The history is not
// touched here: the versions before this one keep their rows, and the new one
// gets its own (StorageFileHistory), so `files_history` stays the record of
// where each version was written (#65).
func (r *sqlRepository) UpdateFile(file File) error {
	r.logger.Debug("updating file: ", slog.Any("file", file))

	exec := "UPDATE files SET name=:name, ext=:ext, path=:path, version=:version, updated_at=NOW() WHERE id=:id"
	result, err := r.db.NamedExec(exec, file)
	if err != nil {
		r.logger.Error("update file table files", slog.Any("error", err))
		return err
	}

	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}

	r.logger.Debug("file updated!")

	return nil
}

// SetFilePublic toggles sharing for a file owned by the caller.
func (r *sqlRepository) SetFilePublic(id uuid.UUID, userID uuid.UUID, isPublic bool) error {
	r.logger.Debug("setting file public", slog.String("id", id.String()), slog.Bool("isPublic", isPublic))

	result, err := r.db.Exec("UPDATE files SET is_public=$3, updated_at=NOW() WHERE id=$1 AND user_id=$2", id, userID, isPublic)
	if err != nil {
		r.logger.Error("set file public", slog.Any("error", err))
		return err
	}

	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *sqlRepository) IsUserOwner(id uuid.UUID, userID uuid.UUID) (bool, error) {
	r.logger.Debug("IsUserOwner", slog.String("id", id.String()), slog.String("userID", userID.String()))

	var count int

	query := "SELECT count(*) FROM files WHERE id=$1 AND user_id=$2"
	if err := r.db.Get(&count, query, id, userID); err != nil {
		r.logger.Error("failed to verify owner file", slog.Any("error", err))
		return false, err
	}

	return count > 0, nil
}

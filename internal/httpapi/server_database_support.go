package httpapi

import (
	"errors"
	"net/http"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/repository"
)

func databaseResourceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrRowHidden):
		writeError(w, http.StatusNotFound, "not_found", "database resource was not found")
		return true
	case errors.Is(err, repository.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to access this database resource")
		return true
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "database resource conflicts with an existing resource")
		return true
	case errors.Is(err, repository.ErrReferenceViolation):
		writeError(w, http.StatusConflict, "reference_violation", "database row is still referenced by another row")
		return true
	case errors.Is(err, repository.ErrSchemaConflict):
		writeError(w, http.StatusConflict, "schema_conflict", "schema change conflicts with existing rows or indexes")
		return true
	case errors.Is(err, repository.ErrUnindexedQuery):
		writeError(w, http.StatusUnprocessableEntity, "unindexed_query", "filters and ordering require a real key index on the declared column")
		return true
	case errors.Is(err, repository.ErrInvalidQuery):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return true
	case errors.Is(err, dbcore.ErrInvalidIdentifier), errors.Is(err, dbcore.ErrInvalidColumn), errors.Is(err, dbcore.ErrInvalidPermissions), errors.Is(err, dbcore.ErrDuplicatePermission), errors.Is(err, dbcore.ErrInvalidName), errors.Is(err, dbcore.ErrInvalidRow), errors.Is(err, dbcore.ErrMissingRequired), errors.Is(err, dbcore.ErrUnknownField), errors.Is(err, dbcore.ErrInvalidValue):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return true
	}
	return false
}

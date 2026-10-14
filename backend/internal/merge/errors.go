package merge

import "errors"

var (
	// ErrAppNotFound — нет цели или источника слияния.
	ErrAppNotFound = errors.New("merge: app not found")
	// ErrNotFound — у цели нет слияния с таким id.
	ErrNotFound = errors.New("merge: not found")
	// ErrConfirmNameMismatch — confirm_name не совпал с именем источника (точно, с учётом регистра).
	ErrConfirmNameMismatch = errors.New("merge: confirm name mismatch")
	// ErrDeletionPending — у цели или источника невыполненная заявка на удаление (приложения, человека или установки).
	ErrDeletionPending = errors.New("merge: deletion pending")
	// ErrAppArchived — цель в архиве: в неё нельзя переносить ключи и события.
	ErrAppArchived = errors.New("merge: target archived")
	// ErrMergeInProgress — цель или источник уже участвуют в слиянии pending, running или failed;
	// для повтора — слияние не в статусе failed.
	ErrMergeInProgress = errors.New("merge: merge in progress")
)

// ValidationError — некорректное поле запроса.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "merge: invalid " + e.Field + ": " + e.Message }

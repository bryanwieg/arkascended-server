package update

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RolloutSteps are the operations Rollout drives; the CLI supplies them.
type RolloutSteps struct {
	Stop         func(ctx context.Context, instance string) error
	Backup       func(ctx context.Context, instance string) (archive string, err error)
	SetImage     func(ctx context.Context, instance, image string) error
	StartAndWait func(ctx context.Context, instance string) error
	Restore      func(ctx context.Context, instance, archive string) error
	Now          func() time.Time
}

// RolloutRequest switches Instance from image From to image To.
type RolloutRequest struct{ StateDir, Instance, From, To string }

// ErrRolledBack means the update failed and the instance is back on its old image.
var ErrRolledBack = errors.New("update rolled back")

// Rollout stops the instance, backs it up, switches it to req.To and waits for
// it to be ready. If the new image fails, it switches back to req.From, restores
// the backup and starts the old image. The attempt is recorded in the state file.
// err is nil on success, wraps ErrRolledBack when the instance is back on From,
// and is any other error when Stop failed (nothing changed) or the rollback failed.
func Rollout(ctx context.Context, req RolloutRequest, steps RolloutSteps) (UpdateAttempt, error) {
	if err := steps.Stop(ctx, req.Instance); err != nil {
		return UpdateAttempt{}, fmt.Errorf("Error: could not stop %s for the update: %w", req.Instance, err)
	}
	attempt := UpdateAttempt{Time: steps.Now(), From: req.From, To: req.To}

	archive, err := steps.Backup(ctx, req.Instance)
	if err != nil {
		err = fmt.Errorf("pre-update backup failed: %w", err)
	} else if err = setPrevious(req); err != nil {
		// Recorded before the switch: starting the new image runs image cleanup,
		// which keeps only compose images and PreviousImage.
		err = fmt.Errorf("could not record the rollback image: %w", err)
	}
	if err != nil {
		reason := err.Error()
		if startErr := steps.StartAndWait(ctx, req.Instance); startErr != nil {
			return failed(req, attempt, ResultRollbackFailed, fmt.Sprintf("%s; restarting %s failed: %v", reason, req.From, startErr))
		}
		return failed(req, attempt, ResultRolledBack, reason)
	}
	attempt.Backup = archive

	switchErr := steps.SetImage(ctx, req.Instance, req.To)
	if switchErr == nil {
		switchErr = steps.StartAndWait(ctx, req.Instance)
	}
	if switchErr == nil {
		attempt.Result = ResultSucceeded
		return attempt, record(req, attempt)
	}

	reason := switchErr.Error()
	if err := steps.SetImage(ctx, req.Instance, req.From); err != nil {
		return failed(req, attempt, ResultRollbackFailed, fmt.Sprintf("%s; rollback failed at switching back to %s: %v", reason, req.From, err))
	}
	_ = steps.Stop(ctx, req.Instance) // the failed container may already be gone
	if err := steps.Restore(ctx, req.Instance, archive); err != nil {
		return failed(req, attempt, ResultRollbackFailed, fmt.Sprintf("%s; rollback failed at restoring %s: %v", reason, archive, err))
	}
	if err := steps.StartAndWait(ctx, req.Instance); err != nil {
		return failed(req, attempt, ResultRollbackFailed, fmt.Sprintf("%s; rollback failed at starting %s: %v", reason, req.From, err))
	}
	return failed(req, attempt, ResultRolledBack, reason)
}

// failed records an unsuccessful attempt and returns the matching error.
func failed(req RolloutRequest, attempt UpdateAttempt, result UpdateResult, message string) (UpdateAttempt, error) {
	attempt.Result = result
	attempt.Message = message
	var err error
	if result == ResultRolledBack {
		err = fmt.Errorf("%w: %s", ErrRolledBack, message)
	} else {
		err = fmt.Errorf("Error: update of %s to %s failed and was not rolled back: %s", req.Instance, req.To, message)
	}
	return attempt, errors.Join(err, record(req, attempt))
}

// setPrevious stores req.From as the instance's rollback image.
func setPrevious(req RolloutRequest) error {
	st, err := LoadState(req.StateDir, req.Instance)
	if err != nil {
		return err
	}
	st.PreviousImage = req.From
	return SaveState(req.StateDir, req.Instance, st)
}

// record saves attempt as the instance's last attempt.
func record(req RolloutRequest, attempt UpdateAttempt) error {
	st, err := LoadState(req.StateDir, req.Instance)
	if err != nil {
		return err
	}
	st.LastAttempt = &attempt
	return SaveState(req.StateDir, req.Instance, st)
}

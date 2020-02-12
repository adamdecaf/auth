// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/go-kit/kit/log"
)

var (
	demoCleanupInterval = func() time.Duration {
		if v := os.Getenv("DEMO_CLEANUP_INTERVAL"); v != "" {
			if dur, err := time.ParseDuration(v); err == nil {
				return dur
			}
		}
		return 24 * time.Hour
	}()
)

func (s *sqliteUserRepository) startAsyncUserCleanup(ctx context.Context, logger log.Logger, interval time.Duration) {
	if interval <= 0*time.Second {
		logger.Log("user-cleanup", "Disabling async user cleanup")
		return
	}

	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			if err := s.cleanup(); err != nil {
				logger.Log("user-cleanup", fmt.Sprintf("error when cleaning up users: %v", err))
			} else {
				logger.Log("user-cleanup", "Done with user cleanup")
			}

		case <-ctx.Done():
			logger.Log("user-cleanup", "Shutting down async user cleanup")
			return
		}
	}
}

func (s *sqliteUserRepository) cleanup() error {
	clean := func(logger log.Logger, db *sql.DB, table string) error {
		stmt, err := s.db.Prepare(fmt.Sprintf(`delete from %s where user_id in (select user_id from users where email like '%%example.com');`, table))
		if err != nil {
			return fmt.Errorf("cleanup: prepare %s: %v", table, err)
		}
		res, err := stmt.Exec()
		if err != nil {
			return fmt.Errorf("cleanup: exec %s: %v", table, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			s.log.Log("cleanup", fmt.Sprintf("deleted %d %s rows", n, table))
		}
		return nil
	}

	if err := clean(s.log, s.db, "user_cookies"); err != nil {
		return err
	}
	if err := clean(s.log, s.db, "user_details"); err != nil {
		return err
	}
	if err := clean(s.log, s.db, "user_passwords"); err != nil {
		return err
	}
	if err := clean(s.log, s.db, "users"); err != nil {
		return err
	}

	// vacuum and reduce disk space.
	stmt, err := s.db.Prepare("vacuum;")
	if err != nil {
		return fmt.Errorf("cleanup: prepare vacuum: %v", err)
	}
	if _, err := stmt.Exec(); err != nil {
		return fmt.Errorf("cleanup: exec vacuum: %v", err)
	}
	return nil
}

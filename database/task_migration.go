package database

import "gorm.io/gorm"

// migrateTasksToSharedStudentScope consolidates legacy per-user duplicate tasks
// and changes uniqueness to one task per student and platform activity.
func migrateTasksToSharedStudentScope() error {
	if !DB.Migrator().HasTable(&Task{}) {
		return nil
	}
	if DB.Migrator().HasIndex(&Task{}, "idx_student_activity_task") && !DB.Migrator().HasIndex(&Task{}, "idx_user_activity_time") {
		return nil
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		ranked := `
			SELECT id, stu_id, activity_id,
			       MAX(id) OVER (PARTITION BY stu_id, activity_id) AS keep_id,
			       COUNT(*) OVER (PARTITION BY stu_id, activity_id) AS duplicate_count,
			       FIRST_VALUE(exec_status) OVER result_order AS latest_status,
			       FIRST_VALUE(retry_count) OVER result_order AS latest_retry_count,
			       FIRST_VALUE(last_error) OVER result_order AS latest_error,
			       FIRST_VALUE(executed_at) OVER result_order AS latest_executed_at
			FROM tasks
			WINDOW result_order AS (PARTITION BY stu_id, activity_id ORDER BY executed_at DESC NULLS LAST, id DESC)
		`
		if err := tx.Exec(`
			WITH ranked AS (` + ranked + `)
			UPDATE tasks AS task
			SET exec_status = ranked.latest_status,
			    retry_count = ranked.latest_retry_count,
			    last_error = ranked.latest_error,
			    executed_at = ranked.latest_executed_at
			FROM ranked
			WHERE task.id = ranked.keep_id AND ranked.duplicate_count > 1
		`).Error; err != nil {
			return err
		}
		return tx.Exec(`
			WITH ranked AS (` + ranked + `)
			DELETE FROM tasks AS task
			USING ranked
			WHERE task.id = ranked.id AND ranked.id <> ranked.keep_id AND ranked.duplicate_count > 1
		`).Error
	}); err != nil {
		return err
	}
	if DB.Migrator().HasIndex(&Task{}, "idx_user_activity_time") {
		if err := DB.Migrator().DropIndex(&Task{}, "idx_user_activity_time"); err != nil {
			return err
		}
	}
	return nil
}

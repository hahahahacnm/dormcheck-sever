package database

// Older versions did not enforce uniqueness for a user-student binding.
func migrateDuplicateStudentBindings() error {
	if !DB.Migrator().HasTable(&UserStudent{}) {
		return nil
	}
	if DB.Migrator().HasIndex(&UserStudent{}, "idx_user_student_binding") {
		return nil
	}
	return DB.Exec(`DELETE FROM user_students WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY user_id, stu_id ORDER BY id) AS position
			FROM user_students
		) duplicates WHERE position > 1
	)`).Error
}

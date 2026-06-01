-- name: GetSetting :one
SELECT setting_value FROM app_settings WHERE setting_key = ?;

-- name: UpsertSetting :exec
INSERT INTO app_settings (setting_key, setting_value) VALUES (?, ?)
ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value);

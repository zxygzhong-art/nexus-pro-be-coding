-- name: UpsertNotification :one
INSERT INTO notifications (
    id, tenant_id, tone, category, title, body, status_text,
    link_url, source_type, source_id, created_by_account_id, created_at, expires_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(tone), sqlc.arg(category),
    sqlc.arg(title), sqlc.arg(body), sqlc.arg(status_text), sqlc.arg(link_url),
    sqlc.arg(source_type), sqlc.arg(source_id), sqlc.arg(created_by_account_id),
    sqlc.arg(created_at), sqlc.arg(expires_at)
)
ON CONFLICT (id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    tone = EXCLUDED.tone,
    category = EXCLUDED.category,
    title = EXCLUDED.title,
    body = EXCLUDED.body,
    status_text = EXCLUDED.status_text,
    link_url = EXCLUDED.link_url,
    source_type = EXCLUDED.source_type,
    source_id = EXCLUDED.source_id,
    created_by_account_id = EXCLUDED.created_by_account_id,
    created_at = EXCLUDED.created_at,
    expires_at = EXCLUDED.expires_at
RETURNING *;

-- name: UpsertNotificationRecipient :one
INSERT INTO notification_recipients (
    notification_id, tenant_id, account_id, read_at, deleted_at, created_at
) VALUES (
    sqlc.arg(notification_id), sqlc.arg(tenant_id), sqlc.arg(account_id),
    sqlc.arg(read_at), sqlc.arg(deleted_at), sqlc.arg(created_at)
)
ON CONFLICT (notification_id, account_id) DO UPDATE SET
    tenant_id = EXCLUDED.tenant_id,
    read_at = EXCLUDED.read_at,
    deleted_at = EXCLUDED.deleted_at,
    created_at = EXCLUDED.created_at
RETURNING *;

-- name: ListNotificationItems :many
SELECT
    n.id,
    n.tone,
    n.category,
    n.title,
    n.body,
    n.status_text,
    n.link_url,
    nr.read_at,
    n.created_at
FROM notification_recipients nr
JOIN notifications n ON n.tenant_id = nr.tenant_id AND n.id = nr.notification_id
WHERE nr.tenant_id = sqlc.arg(tenant_id)
  AND nr.account_id = sqlc.arg(account_id)
  AND nr.deleted_at IS NULL
  AND (n.expires_at IS NULL OR n.expires_at > now())
  AND (sqlc.arg(tone)::text = '' OR n.tone = sqlc.arg(tone))
  AND (NOT sqlc.arg(unread_only)::boolean OR nr.read_at IS NULL)
  AND (
    NOT sqlc.arg(has_cursor)::boolean
    OR n.created_at < sqlc.arg(cursor_created_at)::timestamptz
    OR (n.created_at = sqlc.arg(cursor_created_at)::timestamptz AND n.id < sqlc.arg(cursor_id))
  )
ORDER BY n.created_at DESC, n.id DESC
LIMIT sqlc.arg(limit_count)::int;

-- name: CountUnreadNotifications :one
SELECT count(*)::int
FROM notification_recipients nr
JOIN notifications n ON n.tenant_id = nr.tenant_id AND n.id = nr.notification_id
WHERE nr.tenant_id = sqlc.arg(tenant_id)
  AND nr.account_id = sqlc.arg(account_id)
  AND nr.deleted_at IS NULL
  AND nr.read_at IS NULL
  AND (n.expires_at IS NULL OR n.expires_at > now());

-- name: CountNotificationTones :one
SELECT
    count(*)::int AS all_count,
    count(*) FILTER (WHERE n.tone = 'success')::int AS success_count,
    count(*) FILTER (WHERE n.tone = 'info')::int AS info_count,
    count(*) FILTER (WHERE n.tone = 'warning')::int AS warning_count
FROM notification_recipients nr
JOIN notifications n ON n.tenant_id = nr.tenant_id AND n.id = nr.notification_id
WHERE nr.tenant_id = sqlc.arg(tenant_id)
  AND nr.account_id = sqlc.arg(account_id)
  AND nr.deleted_at IS NULL
  AND (n.expires_at IS NULL OR n.expires_at > now());

-- name: MarkNotificationRead :one
WITH updated AS (
    UPDATE notification_recipients nr
    SET read_at = COALESCE(nr.read_at, sqlc.arg(read_at)::timestamptz)
    WHERE nr.tenant_id = sqlc.arg(tenant_id)
      AND nr.account_id = sqlc.arg(account_id)
      AND nr.notification_id = sqlc.arg(notification_id)
      AND nr.deleted_at IS NULL
    RETURNING notification_id, tenant_id, account_id, read_at
)
SELECT
    n.id,
    n.tone,
    n.category,
    n.title,
    n.body,
    n.status_text,
    n.link_url,
    updated.read_at,
    n.created_at
FROM updated
JOIN notifications n ON n.tenant_id = updated.tenant_id AND n.id = updated.notification_id
WHERE n.expires_at IS NULL OR n.expires_at > now();

-- name: MarkAllNotificationsRead :one
WITH updated AS (
    UPDATE notification_recipients nr
    SET read_at = sqlc.arg(read_at)::timestamptz
    FROM notifications n
    WHERE n.tenant_id = nr.tenant_id
      AND n.id = nr.notification_id
      AND nr.tenant_id = sqlc.arg(tenant_id)
      AND nr.account_id = sqlc.arg(account_id)
      AND nr.deleted_at IS NULL
      AND nr.read_at IS NULL
      AND (n.expires_at IS NULL OR n.expires_at > now())
    RETURNING nr.notification_id
)
SELECT count(*)::int AS updated_count FROM updated;

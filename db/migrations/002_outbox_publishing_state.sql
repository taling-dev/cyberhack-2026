-- 002_outbox_publishing_state.sql
--
-- Adds a 'PUBLISHING' intermediate state to outbox_events.status. Used by the
-- outbox publisher to claim a batch atomically (set PENDING → PUBLISHING)
-- before publishing each message to NATS. After a successful publish, the row
-- transitions to 'PUBLISHED'.
--
-- Recovery: on publisher startup the leader runs `ResetStuckPublishingEvents`,
-- which moves any rows left in PUBLISHING (from a crashed prior leader) back
-- to PENDING so they will be re-claimed and re-published. NATS JetStream's
-- `Nats-Msg-Id` deduplication makes the re-publish a no-op for stream
-- subscribers; core NATS subscribers may see one duplicate per crash, but
-- never a permanently lost or skipped event.
--
-- This migration is safe to apply while the cluster is running: ENUM widening
-- is an instant DDL in TiDB (no row rewrites).

ALTER TABLE outbox_events
  MODIFY COLUMN status ENUM('PENDING','PUBLISHING','PUBLISHED','FAILED') NOT NULL DEFAULT 'PENDING';

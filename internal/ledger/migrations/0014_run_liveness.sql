-- M34's schema delta (SPEC §3, §8; DECISIONS.md ADR-061).
-- Mirrored verbatim in spec/ledger.sql.
--
-- runs gains pid and host: the process executing the run and its hostname,
-- written when the run is created and each time it is resumed. They are for
-- display only. Whether a run's process is alive is its run lock
-- (locks/<run_id>.lock beside the ledger), never these, because a pid is
-- reused after a reboot. Appended last, in §3's column order.
ALTER TABLE runs ADD COLUMN pid INTEGER;
ALTER TABLE runs ADD COLUMN host TEXT;

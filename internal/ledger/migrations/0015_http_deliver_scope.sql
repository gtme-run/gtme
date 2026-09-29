-- M35's data delta (SPEC §3, §8; DECISIONS.md ADR-062). No schema change;
-- noted in spec/ledger.sql's deliveries comment.
--
-- http/deliver now declares idempotency_scope: url, so its deliveries rows
-- are scoped on the step's url as configured (before rendering). Rows
-- written before this migration carry scope ''. Each is backfilled from the
-- url of the http/deliver step in its run's runs.config_json, but only when
-- that run had exactly one http/deliver step: with two or more the row is
-- ambiguous and keeps '' (the next run to each URL may send it once).
UPDATE deliveries
   SET scope = (
     SELECT json_extract(s.value, '$.with.url')
       FROM runs r, json_each(r.config_json, '$.steps') s
      WHERE r.id = deliveries.run_id
        AND json_extract(s.value, '$.use') = 'http/deliver')
 WHERE target = 'http/deliver' AND scope = ''
   AND (SELECT count(*)
          FROM runs r, json_each(r.config_json, '$.steps') s
         WHERE r.id = deliveries.run_id
           AND json_extract(s.value, '$.use') = 'http/deliver') = 1
   AND (SELECT json_type(s.value, '$.with.url')
          FROM runs r, json_each(r.config_json, '$.steps') s
         WHERE r.id = deliveries.run_id
           AND json_extract(s.value, '$.use') = 'http/deliver') = 'text';

-- ADR-060's dispatched events carry the delivery's (target, scope) in their
-- detail, and a later run holds what a dead one left in flight by matching
-- them. They name their step, so the url is unambiguous: backfill every
-- http/deliver dispatched event still scoped ''.
UPDATE step_events
   SET detail = json_set(detail, '$.scope', (
     SELECT json_extract(s.value, '$.with.url')
       FROM runs r, json_each(r.config_json, '$.steps') s
      WHERE r.id = step_events.run_id
        AND json_extract(s.value, '$.id') = step_events.step_id))
 WHERE event = 'dispatched'
   AND json_extract(detail, '$.target') = 'http/deliver'
   AND json_extract(detail, '$.scope') = ''
   AND (SELECT json_type(s.value, '$.with.url')
          FROM runs r, json_each(r.config_json, '$.steps') s
         WHERE r.id = step_events.run_id
           AND json_extract(s.value, '$.id') = step_events.step_id) = 'text';

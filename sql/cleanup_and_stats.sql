-- 2025:
SELECT count(*) FROM presence_logs;
SELECT count(*) FROM presence_logs WHERE timestamp > '2025-09-27 00:00:00';
--DELETE FROM presence_logs WHERE timestamp > '2025-09-27 00:00:00';

-- 2026:
-- The event ended 2026-09-18 midnight / 2026-09-19 morning
-- Backup first: cp /root/chores-data/db.sqlite /root/chores-data/db.sqlite.backup_<date>
SELECT count(*) FROM presence_logs;
SELECT count(*) FROM presence_logs WHERE timestamp > '2026-09-19 00:00:00';
--DELETE FROM presence_logs WHERE timestamp > '2026-09-19 00:00:00';

-- Darik left Tuesday 15.09 at 9:00 local (07:00 UTC):
--DELETE FROM presence_logs WHERE user_id = '392409045517991936' AND timestamp >= '2026-09-15 07:00:00' AND timestamp <= '2026-09-15 08:51:23';

-- Fanda left Tuesday 15.09 at 15:00 local (13:00 UTC) - add missing ticks between 08:51 and 13:00:
--INSERT INTO presence_logs (user_id, timestamp) SELECT '254931468386566144', timestamp FROM presence_logs WHERE user_id = '416902340696604674' AND timestamp > '2026-09-15 08:51:23' AND timestamp <= '2026-09-15 13:00:00';

-- Anetka (user_id '1393320844414947481') excluded from chores statistics.

--VACUUM;

SELECT avg(time_spent_min) FROM work_logs;
SELECT median(time_spent_min) FROM work_logs;

SELECT ca.user_id, count(*) as total_assignments FROM chore_assignments as ca GROUP BY ca.user_id;
SELECT ca.user_id, count(*) as refused_assignments FROM chore_assignments as ca WHERE ca.refused is not NULL GROUP BY ca.user_id;
SELECT ca.user_id, count(*) as timeouted_assignments FROM chore_assignments as ca WHERE ca.timeouted is not NULL GROUP BY ca.user_id;
SELECT ca.user_id, count(*) as acked_assignments FROM chore_assignments as ca WHERE ca.acked is not NULL GROUP BY ca.user_id;
SELECT ca.user_id, count(*) as bailed_assignments FROM chore_assignments as ca WHERE ca.timeouted is not NULL or ca.refused is not NULL GROUP BY ca.user_id;

SELECT avg(wl.time_spent_min - c.estimated_time_min) as time_adjustment_min FROM work_logs as wl JOIN chores as c ON c.id = wl.chore_id WHERE c.cancelled is NULL;
SELECT wl.user_id, sum(wl.time_spent_min - c.estimated_time_min) as time_adjustment_min FROM work_logs as wl JOIN chores as c ON c.id = wl.chore_id WHERE c.cancelled is NULL GROUP BY wl.user_id;

SELECT c.name, sum(wl.time_spent_min) FROM chores as c JOIN work_logs as wl ON c.id = wl.chore_id WHERE c.cancelled is NULL GROUP BY c.id;
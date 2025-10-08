SELECT count(*) FROM presence_logs;
SELECT count(*) FROM presence_logs WHERE timestamp > '2025-09-27 00:00:00';

--DELETE FROM presence_logs WHERE timestamp > '2025-09-27 00:00:00';

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
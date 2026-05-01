ALTER TABLE execution_records
    ADD CONSTRAINT fk_execution_records_schedule_id
    FOREIGN KEY (schedule_id) REFERENCES schedules(id) ON DELETE SET NULL;

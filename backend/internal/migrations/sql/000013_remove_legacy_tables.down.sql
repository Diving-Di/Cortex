DO $$
BEGIN
    RAISE EXCEPTION 'migration 000013 is irreversible: legacy table data cannot be restored';
END
$$;

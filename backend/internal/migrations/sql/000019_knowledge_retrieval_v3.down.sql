DO $$
BEGIN
    RAISE EXCEPTION 'migration 000019 cannot roll back activated knowledge index versions';
END $$;

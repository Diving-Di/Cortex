DO $$
BEGIN
    RAISE EXCEPTION
        'migration 000012 is irreversible because it permanently deletes personal knowledge data';
END
$$;

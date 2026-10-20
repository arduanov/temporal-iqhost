-- +goose Up
ALTER TABLE gpu_devices ADD CONSTRAINT gpu_node_pair UNIQUE(id,node_id);
ALTER TABLE compute_allocations ADD CONSTRAINT allocation_gpu_node FOREIGN KEY(gpu_id,node_id) REFERENCES gpu_devices(id,node_id);
-- +goose StatementBegin
CREATE FUNCTION check_ip_pool() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM ip_pools WHERE id=NEW.pool_id AND NEW.address <<= subnet) THEN
    RAISE EXCEPTION 'address outside pool' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER ip_pool_membership BEFORE INSERT OR UPDATE OF pool_id,address ON ip_addresses FOR EACH ROW EXECUTE FUNCTION check_ip_pool();
-- +goose StatementBegin
CREATE FUNCTION check_pool_subnet() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS(SELECT 1 FROM ip_addresses WHERE pool_id=NEW.id AND NOT(address <<= NEW.subnet)) THEN
    RAISE EXCEPTION 'pool excludes existing address' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER pool_subnet_membership BEFORE UPDATE OF subnet ON ip_pools FOR EACH ROW EXECUTE FUNCTION check_pool_subnet();
-- +goose Down
DROP TRIGGER pool_subnet_membership ON ip_pools;
DROP FUNCTION check_pool_subnet();
DROP TRIGGER ip_pool_membership ON ip_addresses;
DROP FUNCTION check_ip_pool();
ALTER TABLE compute_allocations DROP CONSTRAINT allocation_gpu_node;
ALTER TABLE gpu_devices DROP CONSTRAINT gpu_node_pair;

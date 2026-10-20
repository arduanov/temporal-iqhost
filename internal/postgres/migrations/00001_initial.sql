-- +goose Up
CREATE TABLE compute_nodes (id bigserial PRIMARY KEY, name text UNIQUE NOT NULL, cpu integer NOT NULL CHECK(cpu>0), memory_mib integer NOT NULL CHECK(memory_mib>0), available boolean NOT NULL DEFAULT true);
CREATE TABLE gpu_devices (id bigserial PRIMARY KEY, node_id bigint NOT NULL REFERENCES compute_nodes, model text NOT NULL, available boolean NOT NULL DEFAULT true);
CREATE TABLE storage_pools (id bigserial PRIMARY KEY, name text UNIQUE NOT NULL, capacity_gib integer NOT NULL CHECK(capacity_gib>0), available boolean NOT NULL DEFAULT true);
CREATE TABLE ip_pools (id bigserial PRIMARY KEY, name text UNIQUE NOT NULL, subnet cidr NOT NULL CHECK(family(subnet)=4));
CREATE TABLE vms (id text PRIMARY KEY, cpu integer NOT NULL CHECK(cpu>0), memory_mib integer NOT NULL CHECK(memory_mib>0), gpu boolean NOT NULL, disk_gib integer NOT NULL CHECK(disk_gib>0), state text NOT NULL DEFAULT 'not-created' CHECK(state IN ('not-created','stopped','running','deleted','unknown')), current_operation_id uuid, recovery_required boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE vm_operations (id uuid PRIMARY KEY, vm_id text NOT NULL REFERENCES vms, user_id text NOT NULL, kind text NOT NULL CHECK(kind IN ('create','start','stop','delete','cleanup')), request jsonb NOT NULL, status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','succeeded','failed','canceled','rollback-blocked')), phase text NOT NULL DEFAULT 'forward' CHECK(phase IN ('forward','rollback')), step text NOT NULL DEFAULT '', workflow_id text UNIQUE NOT NULL, run_id text NOT NULL DEFAULT '', error text NOT NULL DEFAULT '', recovery_target text NOT NULL CHECK(recovery_target IN ('not-created','stopped','deleted')), source_operation_id uuid REFERENCES vm_operations, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE vms ADD FOREIGN KEY(current_operation_id) REFERENCES vm_operations;
CREATE UNIQUE INDEX one_active_operation ON vm_operations(vm_id) WHERE status IN ('pending','running');
CREATE TABLE ip_addresses (id bigserial PRIMARY KEY, pool_id bigint NOT NULL REFERENCES ip_pools, address inet UNIQUE NOT NULL CHECK(family(address)=4), vm_id text UNIQUE REFERENCES vms, assigned_at timestamptz);
CREATE TABLE vm_disks (id bigserial PRIMARY KEY, vm_id text NOT NULL REFERENCES vms, pool_id bigint NOT NULL REFERENCES storage_pools, size_gib integer NOT NULL CHECK(size_gib>0), state text NOT NULL DEFAULT 'allocated' CHECK(state IN ('allocated','deleted')), created_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE UNIQUE INDEX one_disk ON vm_disks(vm_id) WHERE deleted_at IS NULL;
CREATE INDEX active_disks ON vm_disks(pool_id) WHERE deleted_at IS NULL;
CREATE TABLE compute_allocations (id bigserial PRIMARY KEY, vm_id text NOT NULL REFERENCES vms, node_id bigint NOT NULL REFERENCES compute_nodes, cpu integer NOT NULL CHECK(cpu>0), memory_mib integer NOT NULL CHECK(memory_mib>0), gpu_id bigint REFERENCES gpu_devices, operation_id uuid NOT NULL REFERENCES vm_operations, allocated_at timestamptz NOT NULL DEFAULT now(), released_at timestamptz);
CREATE UNIQUE INDEX one_compute ON compute_allocations(vm_id) WHERE released_at IS NULL;
CREATE UNIQUE INDEX one_gpu ON compute_allocations(gpu_id) WHERE released_at IS NULL;
CREATE INDEX active_compute ON compute_allocations(node_id) WHERE released_at IS NULL;
CREATE TABLE operation_steps (operation_id uuid NOT NULL REFERENCES vm_operations, step text NOT NULL, result jsonb NOT NULL DEFAULT '{}', completed_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(operation_id,step));
CREATE TABLE outbox (id bigserial PRIMARY KEY, operation_id uuid UNIQUE NOT NULL REFERENCES vm_operations, attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(), lease_token uuid, locked_until timestamptz, delivered_at timestamptz, last_error text NOT NULL DEFAULT '');
CREATE INDEX outbox_pending ON outbox(next_attempt_at,id) WHERE delivered_at IS NULL;
-- +goose Down
DROP TABLE outbox, operation_steps, compute_allocations, vm_disks, ip_addresses;
ALTER TABLE vms DROP CONSTRAINT vms_current_operation_id_fkey;
DROP TABLE vm_operations, vms, ip_pools, storage_pools, gpu_devices, compute_nodes;

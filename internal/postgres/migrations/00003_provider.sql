-- +goose Up
ALTER TABLE vms ADD COLUMN generation bigint NOT NULL DEFAULT 0 CHECK (generation>=0);
CREATE TABLE confirmed_instances (vm_id text PRIMARY KEY REFERENCES vms, node_id bigint REFERENCES compute_nodes, exists boolean NOT NULL DEFAULT false, network_attached boolean NOT NULL DEFAULT false, powered_on boolean NOT NULL DEFAULT false, owner_operation_id uuid NOT NULL REFERENCES vm_operations, generation bigint NOT NULL DEFAULT 0, updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE infrastructure_commands (
 command_key text PRIMARY KEY,
 operation_id uuid NOT NULL REFERENCES vm_operations,
 vm_id text NOT NULL REFERENCES vms,
 generation bigint NOT NULL,
 request jsonb NOT NULL,
 result jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE infrastructure_commands, confirmed_instances;
ALTER TABLE vms DROP COLUMN generation;

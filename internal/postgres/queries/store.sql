-- name: LockVM :one
SELECT * FROM vms WHERE id=$1 FOR UPDATE;
-- name: GetVM :one
SELECT * FROM vms WHERE id=$1;
-- name: GetOperation :one
SELECT * FROM vm_operations WHERE id=$1;
-- name: LockOperation :one
SELECT * FROM vm_operations WHERE id=$1 FOR UPDATE;
-- name: InsertVM :exec
INSERT INTO vms(id,cpu,memory_mib,gpu,disk_gib) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: InsertOperation :one
INSERT INTO vm_operations(id,vm_id,kind,request,workflow_id,recovery_target,source_operation_id,user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
-- name: OwnVM :exec
UPDATE vms SET current_operation_id=$2,generation=generation+1,updated_at=now() WHERE id=$1;
-- name: Enqueue :exec
INSERT INTO outbox(operation_id) VALUES($1);
-- name: ListVMs :many
SELECT id FROM vms ORDER BY id;
-- name: VMView :one
SELECT jsonb_build_object('vm',to_jsonb(v),'operation',(SELECT to_jsonb(o) FROM vm_operations o WHERE o.id=v.current_operation_id),'compute',(SELECT to_jsonb(a) FROM compute_allocations a WHERE a.vm_id=v.id AND released_at IS NULL),'disk',(SELECT to_jsonb(d) FROM vm_disks d WHERE d.vm_id=v.id AND deleted_at IS NULL),'ip',(SELECT host(address) FROM ip_addresses WHERE vm_id=v.id)) AS result FROM vms v WHERE v.id=$1;
-- name: LockNodes :many
SELECT * FROM compute_nodes ORDER BY id FOR UPDATE;
-- name: LockGPUs :many
SELECT * FROM gpu_devices ORDER BY id FOR UPDATE;
-- name: LockStorage :many
SELECT * FROM storage_pools ORDER BY id FOR UPDATE;
-- name: LockIPs :many
SELECT * FROM ip_addresses ORDER BY id FOR UPDATE;
-- name: ComputeUsage :one
SELECT coalesce(sum(cpu),0)::bigint AS cpu, coalesce(sum(memory_mib),0)::bigint AS memory_mib FROM compute_allocations WHERE node_id=$1 AND released_at IS NULL;
-- name: GPUUsed :one
SELECT EXISTS(SELECT 1 FROM compute_allocations WHERE gpu_id=$1 AND released_at IS NULL);
-- name: DiskUsage :one
SELECT coalesce(sum(size_gib),0)::bigint FROM vm_disks WHERE pool_id=$1 AND deleted_at IS NULL;
-- name: AllocateCompute :exec
INSERT INTO compute_allocations(vm_id,node_id,cpu,memory_mib,gpu_id,operation_id) VALUES($1,$2,$3,$4,$5,$6);
-- name: AllocateDisk :exec
INSERT INTO vm_disks(vm_id,pool_id,size_gib) VALUES($1,$2,$3);
-- name: AssignIP :exec
UPDATE ip_addresses SET vm_id=$2,assigned_at=now() WHERE id=$1;
-- name: GetCompute :one
SELECT * FROM compute_allocations WHERE vm_id=$1 AND released_at IS NULL;
-- name: Instance :one
SELECT * FROM confirmed_instances WHERE vm_id=$1;
-- name: PutInstance :exec
INSERT INTO confirmed_instances(vm_id,node_id,exists,network_attached,powered_on,owner_operation_id,generation) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(vm_id) DO UPDATE SET node_id=excluded.node_id,exists=excluded.exists,network_attached=excluded.network_attached,powered_on=excluded.powered_on,owner_operation_id=excluded.owner_operation_id,generation=excluded.generation,updated_at=now();
-- name: ReleaseCompute :exec
UPDATE compute_allocations SET released_at=now() WHERE vm_id=$1 AND released_at IS NULL;
-- name: ReleaseDisk :exec
UPDATE vm_disks SET state='deleted',deleted_at=now() WHERE vm_id=$1 AND deleted_at IS NULL;
-- name: ReleaseIP :exec
UPDATE ip_addresses SET vm_id=NULL,assigned_at=NULL WHERE vm_id=$1;
-- name: StepDone :one
SELECT EXISTS(SELECT 1 FROM operation_steps WHERE operation_id=$1 AND step=$2);
-- name: SaveStep :exec
INSERT INTO operation_steps(operation_id,step) VALUES($1,$2);
-- name: Progress :exec
UPDATE vm_operations SET status='running',step=$2,updated_at=now() WHERE id=$1;
-- name: Rollback :exec
UPDATE vm_operations SET phase='rollback',updated_at=now() WHERE id=$1;
-- name: FinishOperation :exec
UPDATE vm_operations SET status=$2,error=$3,updated_at=now() WHERE id=$1;
-- name: SetVMState :exec
UPDATE vms SET state=$2,recovery_required=$3,updated_at=now() WHERE id=$1;
-- name: Claim :one
UPDATE outbox SET lease_token=$1,locked_until=now()+interval '30 seconds' WHERE id=(SELECT b.id FROM outbox b JOIN vm_operations o ON o.id=b.operation_id WHERE delivered_at IS NULL AND next_attempt_at<=now() AND (locked_until IS NULL OR locked_until<now()) AND o.status IN ('pending','running') ORDER BY b.next_attempt_at,b.id FOR UPDATE OF b SKIP LOCKED LIMIT 1) RETURNING *;
-- name: Delivered :one
UPDATE outbox SET delivered_at=now(),locked_until=NULL WHERE id=$1 AND lease_token=$2 RETURNING operation_id;
-- name: SetRunID :exec
UPDATE vm_operations SET run_id=$2 WHERE id=$1;
-- name: RetryDelivery :exec
UPDATE outbox SET attempts=attempts+1,next_attempt_at=$3,locked_until=NULL,last_error=$4 WHERE id=$1 AND lease_token=$2;
-- name: ReconcileCandidates :many
SELECT o.* FROM vm_operations o JOIN outbox b ON b.operation_id=o.id WHERE o.status IN ('pending','running') AND b.delivered_at IS NOT NULL ORDER BY o.id;
-- name: SeedNodes :exec
INSERT INTO compute_nodes(id,name,cpu,memory_mib) VALUES(1,'node-1',8,16384),(2,'node-2',8,16384) ON CONFLICT DO NOTHING;
-- name: SeedGPUs :exec
INSERT INTO gpu_devices(id,node_id,model) VALUES(1,1,'demo-gpu'),(2,2,'demo-gpu') ON CONFLICT DO NOTHING;
-- name: SeedStorage :exec
INSERT INTO storage_pools(id,name,capacity_gib) VALUES(1,'shared',200) ON CONFLICT DO NOTHING;
-- name: SeedIPPool :exec
INSERT INTO ip_pools(id,name,subnet) VALUES(1,'documentation','192.0.2.0/24') ON CONFLICT DO NOTHING;
-- name: SeedIPs :exec
INSERT INTO ip_addresses(pool_id,address) SELECT 1,'192.0.2.10'::inet+n FROM generate_series(0,9) n ON CONFLICT DO NOTHING;
-- name: SaveStepResult :exec
UPDATE operation_steps SET result=$3 WHERE operation_id=$1 AND step=$2;
-- name: StepResult :one
SELECT result FROM operation_steps WHERE operation_id=$1 AND step=$2;
-- name: ResourcesPresent :one
SELECT EXISTS(SELECT 1 FROM compute_allocations a WHERE a.vm_id=$1 AND a.released_at IS NULL) AS compute,
EXISTS(SELECT 1 FROM vm_disks d WHERE d.vm_id=$1 AND d.deleted_at IS NULL) AS disk,
EXISTS(SELECT 1 FROM ip_addresses i WHERE i.vm_id=$1) AS ip;

-- name: BumpGeneration :exec
UPDATE vms SET generation=generation+1 WHERE id=$1;
-- name: GetDisk :one
SELECT * FROM vm_disks WHERE vm_id=$1 AND deleted_at IS NULL;
-- name: GetIP :one
SELECT host(address)::text FROM ip_addresses WHERE vm_id=$1;
-- name: InsertCommand :exec
INSERT INTO infrastructure_commands(command_key,operation_id,vm_id,generation,request) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: GetCommand :one
SELECT * FROM infrastructure_commands WHERE command_key=$1;
-- name: CompleteCommand :exec
UPDATE infrastructure_commands SET result=$2 WHERE command_key=$1;

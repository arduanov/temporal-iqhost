-- name: AdminVMs :many
SELECT machine.id, machine.cpu, machine.memory_mib, machine.disk_gib, machine.gpu,
 machine.state, machine.recovery_required,
 coalesce(machine.current_operation_id::text, '')::text AS operation_id,
 coalesce(operation.status, '') AS operation_status,
 coalesce(operation.recovery_target, '') AS recovery_target,
 coalesce(node.name, '') AS node,
 coalesce(host(ip.address), '')::text AS ip,
 coalesce(allocation.gpu_id, 0)::bigint AS gpu_id,
 coalesce(disk.id, 0)::bigint AS disk_id
FROM vms machine
LEFT JOIN vm_operations operation ON operation.id = machine.current_operation_id
LEFT JOIN compute_allocations allocation ON allocation.vm_id = machine.id AND allocation.released_at IS NULL
LEFT JOIN compute_nodes node ON node.id = allocation.node_id
LEFT JOIN vm_disks disk ON disk.vm_id = machine.id AND disk.deleted_at IS NULL
LEFT JOIN ip_addresses ip ON ip.vm_id = machine.id
WHERE (sqlc.arg(search)::text = '' OR strpos(machine.id, sqlc.arg(search)) > 0)
AND (sqlc.arg(exact_id)::text = '' OR machine.id = sqlc.arg(exact_id))
AND (sqlc.arg(state)::text = '' OR machine.state = sqlc.arg(state))
AND (NOT sqlc.arg(recovery_only)::boolean OR machine.recovery_required)
ORDER BY machine.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: AdminOperations :many
SELECT * FROM vm_operations
WHERE (sqlc.arg(vm_id)::text = '' OR vm_id = sqlc.arg(vm_id))
AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
AND (sqlc.arg(user_id)::text = '' OR user_id = sqlc.arg(user_id))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: AdminDelivery :one
SELECT attempts, next_attempt_at, delivered_at FROM outbox WHERE operation_id = $1;

-- name: AdminSteps :many
SELECT step, completed_at FROM operation_steps WHERE operation_id = $1 ORDER BY completed_at, step;

-- name: AdminNodes :many
SELECT node.id, node.name, node.available, node.cpu, node.memory_mib,
 coalesce(sum(allocation.cpu), 0)::bigint AS reserved_cpu,
 coalesce(sum(allocation.memory_mib), 0)::bigint AS reserved_memory_mib,
 coalesce(jsonb_agg(allocation.vm_id ORDER BY allocation.vm_id) FILTER (WHERE allocation.id IS NOT NULL), '[]'::jsonb)::text AS vm_ids
FROM compute_nodes node
LEFT JOIN compute_allocations allocation ON allocation.node_id = node.id AND allocation.released_at IS NULL
GROUP BY node.id ORDER BY node.id;

-- name: AdminGPUs :many
SELECT gpu.id, gpu.model, node.name AS node, gpu.available, node.available AS node_available,
 coalesce(allocation.vm_id, '') AS vm_id
FROM gpu_devices gpu JOIN compute_nodes node ON node.id = gpu.node_id
LEFT JOIN compute_allocations allocation ON allocation.gpu_id = gpu.id AND allocation.released_at IS NULL
ORDER BY gpu.id;

-- name: AdminStorage :many
SELECT pool.id, pool.name, pool.available, pool.capacity_gib,
 coalesce(sum(disk.size_gib), 0)::bigint AS reserved_gib,
 coalesce(jsonb_agg(jsonb_build_object('id', disk.id, 'vm_id', disk.vm_id, 'size_gib', disk.size_gib) ORDER BY disk.id) FILTER (WHERE disk.id IS NOT NULL), '[]'::jsonb)::text AS disks
FROM storage_pools pool LEFT JOIN vm_disks disk ON disk.pool_id = pool.id AND disk.deleted_at IS NULL
GROUP BY pool.id ORDER BY pool.id;

-- name: AdminIPPools :many
SELECT pool.id, pool.name, pool.subnet::text AS subnet, count(ip.id)::bigint AS total,
 count(ip.vm_id)::bigint AS reserved,
 coalesce(jsonb_agg(jsonb_build_object('address', host(ip.address), 'vm_id', coalesce(ip.vm_id, '')) ORDER BY ip.address) FILTER (WHERE ip.id IS NOT NULL), '[]'::jsonb)::text AS addresses
FROM ip_pools pool LEFT JOIN ip_addresses ip ON ip.pool_id = pool.id
GROUP BY pool.id ORDER BY pool.id;

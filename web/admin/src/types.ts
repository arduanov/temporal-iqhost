export type Kind = 'create' | 'start' | 'stop' | 'delete' | 'cleanup'
export interface Command {
  vm_id: string
  user_id: string
  kind: Kind
  cpu?: number
  memory_mib?: number
  disk_gib?: number
  gpu?: boolean
}
export interface CommandRequest {
  request_id: string
  command: Command
}
export interface Machine {
  id: string
  cpu: number
  memory_mib: number
  disk_gib: number
  gpu: boolean
  state: string
  recovery_required: boolean
  operation_id: string
  operation_status: string
  recovery_target: string
  node: string
  ip: string
  gpu_id: number
  disk_id: number
}
export interface Operation {
  id: string
  vm_id: string
  user_id: string
  kind: Kind
  status: string
  phase: string
  step: string
  workflow_id: string
  run_id: string
  error: string
  recovery_target: string
  source_operation_id?: string
  created_at: string
  updated_at: string
}
export interface OperationDetail extends Operation {
  delivery: { attempts: number; next_attempt_at: string; delivered_at: string | null }
  steps: { step: string; completed_at: string }[]
}
export interface Page<T> {
  items: T[]
  limit: number
  offset: number
}
export interface Resources {
  nodes: {
    id: number
    name: string
    available: boolean
    cpu: number
    memory_mib: number
    reserved_cpu: number
    reserved_memory_mib: number
    free_cpu: number
    free_memory_mib: number
    vm_ids: string[]
  }[]
  gpus: { id: number; model: string; node: string; available: boolean; vm_id: string }[]
  storage: {
    id: number
    name: string
    available: boolean
    capacity_gib: number
    reserved_gib: number
    free_gib: number
    disks: { id: number; vm_id: string; size_gib: number }[]
  }[]
  ip_pools: {
    id: number
    name: string
    subnet: string
    total: number
    reserved: number
    free: number
    addresses: { address: string; vm_id: string }[]
  }[]
}

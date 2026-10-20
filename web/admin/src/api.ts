export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message)
  }
}
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch('/api/v1' + path, {
    ...options,
    signal: options.signal ?? AbortSignal.timeout(20000),
  })
  const body = await response.json()
  if (!response.ok) throw new APIError(body.error || 'Ошибка запроса', response.status)
  return body as T
}
export function query(values: Record<string, string | number | boolean>): string {
  return new URLSearchParams(
    Object.entries(values).map(([key, value]) => [key, String(value)]),
  ).toString()
}

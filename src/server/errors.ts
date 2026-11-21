export class AppError extends Error {
  constructor(message: string, public status = 422, public errorClass = 'validation', public providerRequestId?:string) { super(message); this.name = 'AppError'; }
}
export function assert(condition: unknown, message: string, status = 422, errorClass = 'validation'): asserts condition { if (!condition) throw new AppError(message, status, errorClass); }
export function publicError(error: unknown) { return error instanceof AppError ? { status: error.status, error: error.message, errorClass: error.errorClass } : { status: 500, error: '本地服务未能完成操作，请重试或查看诊断记录。', errorClass: 'storage_error' }; }

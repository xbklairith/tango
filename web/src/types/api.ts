export interface ApiError {
  error: string;
  code: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: {
    total: number;
    offset: number;
    limit: number;
  };
}

export interface PaginationParams {
  offset?: number;
  limit?: number;
}

import { DataQuery, DataSourceJsonData } from '@grafana/data';

export interface MyQuery extends DataQuery {
  queryText?: string;
  constant: number;
  service: string;
  keyword: string;
}

export const defaultQuery: Partial<MyQuery> = {
  constant: 6.5,
};

/**
 * These are options configured for each DataSource instance
 */
export interface MyDataSourceOptions extends DataSourceJsonData {
  server?: string;
  port?: string;
  role?: string;
  database?: string;
  metatable?: string;
}

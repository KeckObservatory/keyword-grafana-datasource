import { DataQuery, DataSourceJsonData } from '@grafana/data';

export interface KeywordQuery extends DataQuery {
  queryText: string;
  service: string;
  keyword: string;
  unitConversion: number;
  transform: number;
}

export const defaultQuery: Partial<KeywordQuery> = {
  unitConversion: 0,
  transform: 0,
};

/**
 * These are options configured for each DataSource instance
 */
export interface KeywordDataSourceOptions extends DataSourceJsonData {
  server: string;
  port: string;
  role: string;
  database: string;
  metatable: string;
}

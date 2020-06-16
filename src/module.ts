import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './DataSource';
import { ConfigEditor } from './ConfigEditor';
import { QueryEditor } from './QueryEditor';
import { KeywordQuery, KeywordDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, KeywordQuery, KeywordDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);

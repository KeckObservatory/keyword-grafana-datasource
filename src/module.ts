import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './DataSource';
import { ConfigEditor } from 'components/ConfigEditor';
import { QueryEditor } from 'components/QueryEditor';
import { KeywordQuery, KeywordDataSourceOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, KeywordQuery, KeywordDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);

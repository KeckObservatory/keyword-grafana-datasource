import { DataSourceInstanceSettings, SelectableValue } from '@grafana/data';
import { DataSourceWithBackend } from '@grafana/runtime';
import { MyDataSourceOptions, MyQuery } from './types';

export class DataSource extends DataSourceWithBackend<MyQuery, MyDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<MyDataSourceOptions>) {
    super(instanceSettings);
  }

  async getServices(): Promise<Array<SelectableValue<string>>> {
    return this.getResource('services').then(({ services }) =>
      services ? Object.entries(services).map(([value, label]) => ({ label, value } as SelectableValue<string>)) : []
    );
  }

  async getKeywords(service: string): Promise<Array<SelectableValue<string>>> {
    return this.getResource('keywords', { service: service }).then(({ keywords }) =>
      keywords ? Object.entries(keywords).map(([value, label]) => ({ label, value } as SelectableValue<string>)) : []
    );
  }
}

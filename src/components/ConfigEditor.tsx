import React, { ChangeEvent, PureComponent } from 'react';
import { LegacyForms } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { KeywordDataSourceOptions } from '../types';

const { FormField } = LegacyForms;

interface Props extends DataSourcePluginOptionsEditorProps<KeywordDataSourceOptions> {}

interface State {}

export class ConfigEditor extends PureComponent<Props, State> {
  onServerChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      server: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  onPortChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      port: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  onRoleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      role: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  onDatabaseChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      database: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  onMetatableChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      metatable: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  render() {
    const { options } = this.props;
    const { jsonData } = options;

    return (
      <div className="gf-form-group">
        <div className="gf-form">
          <FormField
            label="Database server"
            labelWidth={10}
            inputWidth={20}
            onChange={this.onServerChange}
            value={jsonData.server || ''}
            placeholder="vm-history-1"
          />
          <FormField
            label="Port"
            labelWidth={3}
            inputWidth={4}
            onChange={this.onPortChange}
            value={jsonData.port || ''}
            placeholder="5432"
          />
        </div>
        <div className="gf-form">
          <FormField
            label="Role"
            labelWidth={10}
            inputWidth={20}
            onChange={this.onRoleChange}
            value={jsonData.role || ''}
            placeholder="turk"
          />
        </div>
        <div className="gf-form">
          <FormField
            label="Database"
            labelWidth={10}
            inputWidth={20}
            onChange={this.onDatabaseChange}
            value={jsonData.database || ''}
            placeholder="keywordlog"
          />
        </div>
        <div className="gf-form">
          <FormField
            label="Meta table"
            labelWidth={10}
            inputWidth={20}
            onChange={this.onMetatableChange}
            value={jsonData.metatable || ''}
            placeholder="ktlmeta"
          />
        </div>
      </div>
    );
  }
}

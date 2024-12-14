import defaults from 'lodash/defaults';

import React, { PureComponent } from 'react';
import { InlineFormLabel, SegmentAsync, Select } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../DataSource';
import { defaultQuery, KeywordDataSourceOptions, KeywordQuery } from '../types';

type Props = QueryEditorProps<DataSource, KeywordQuery, KeywordDataSourceOptions>;

export class QueryEditor extends PureComponent<Props> {
  onServiceChange = (item: any) => {
    const { onChange, query } = this.props;
    // Repopulate the keyword list based on the service selected
    onChange({ ...query, service: item.value });
  };

  onKeywordChange = (item: any) => {
    const { query, onRunQuery, onChange } = this.props;

    if (!item.value) {
      return; // ignore delete
    }

    query.keyword = item.value;
    query.queryText = query.service + '.' + query.keyword;
    onChange({ ...query, keyword: item.value });
    onRunQuery();
  };

  unitConversionOptions = [
    { label: '(none)', value: 0 },
    { label: 'degrees to radians', value: 1 },
    { label: 'radians to degrees', value: 2 },
    { label: 'radians to arcseconds', value: 3 },
    { label: 'Kelvin to Celcius', value: 4 },
    { label: 'Celcius to Kelvin', value: 5 },
  ];

  onUnitConversionChange = (item: any) => {
    const { onChange, query, onRunQuery } = this.props;
    onChange({ ...query, unitConversion: item.value });
    onRunQuery();
  };

  transformOptions = [
    { label: '(none)', value: 0 },
    { label: '1st derivative (no rounding)', value: 1 },
    { label: '1st derivative (1Hz rounding)', value: 2 },
    { label: '1st derivative (10Hz rounding)', value: 3 },
    { label: '1st derivative (100Hz rounding)', value: 4 },
    { label: 'delta', value: 5 },
  ];

  onTransformChange = (item: any) => {
    const { onChange, query, onRunQuery } = this.props;
    onChange({ ...query, transform: item.value });
    onRunQuery();
  };

  render() {
    const datasource = this.props.datasource;
    const query = defaults(this.props.query, defaultQuery);

    // noinspection CheckTagEmptyBody
    return (
      <>
        <div className="gf-form-inline">
          <InlineFormLabel width={10} className="query-keyword" tooltip={<p>Select a keyword.</p>}>
            Keyword selection
          </InlineFormLabel>
          <SegmentAsync
            loadOptions={() => datasource.getServices()}
            placeholder="(select a service)"
            value={query.service}
            allowCustomValue={false}
            onChange={this.onServiceChange}
          ></SegmentAsync>
          <SegmentAsync
            loadOptions={() => datasource.getKeywords(query.service)}
            placeholder="(select a keyword))"
            value={query.keyword}
            allowCustomValue={false}
            onChange={this.onKeywordChange}
          ></SegmentAsync>
        </div>
        <div className="gf-form-inline">
          <InlineFormLabel width={10} className="convert-units" tooltip={<p>Convert units.</p>}>
            Units conversion
          </InlineFormLabel>
          <Select
            width={30}
            placeholder={'(none)'}
            defaultValue={0}
            options={this.unitConversionOptions}
            value={query.unitConversion}
            allowCustomValue={false}
            onChange={this.onUnitConversionChange}
          />
        </div>
        <div className="gf-form-inline">
          <InlineFormLabel width={10} className="transform" tooltip={<p>Transform data.</p>}>
            Transform
          </InlineFormLabel>
          <Select
            width={30}
            placeholder={'(none)'}
            defaultValue={0}
            options={this.transformOptions}
            value={query.transform}
            allowCustomValue={false}
            onChange={this.onTransformChange}
          />
        </div>
      </>
    );
  }
}

import defaults from 'lodash/defaults';

import React, { PureComponent } from 'react';
import { InlineFormLabel, SegmentAsync, Select } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from './DataSource';
import { defaultQuery, KeywordDataSourceOptions, KeywordQuery } from './types';

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
    { label: 'none', value: 0 },
    { label: 'degrees to radians', value: 1 },
    { label: 'radians to degrees', value: 2 },
    { label: 'Kelvin to Celcius', value: 3 },
    { label: 'Celcius to Kelvin', value: 4 },
  ];

  onUnitConversionChange = (item: any) => {
    const { onChange, query, onRunQuery } = this.props;
    onChange({ ...query, unitConversion: item.value });
    onRunQuery();
  };

  render() {
    const datasource = this.props.datasource;
    const query = defaults(this.props.query, defaultQuery);

    // noinspection CheckTagEmptyBody
    return (
      <>
        <div className="gf-form-inline">
          <InlineFormLabel
            width={10}
            className="query-keyword"
            tooltip={
              <p>
                Select a <code>keyword</code>.
              </p>
            }
          >
            Keyword selection
          </InlineFormLabel>
          <SegmentAsync
            loadOptions={() => datasource.getServices()}
            placeholder="dcs1"
            value={query.service}
            allowCustomValue={false}
            onChange={this.onServiceChange}
          ></SegmentAsync>
          <SegmentAsync
            loadOptions={() => datasource.getKeywords(query.service)}
            placeholder="PRIMTEMP"
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
            width={20}
            placeholder={'none'}
            defaultValue={0}
            options={this.unitConversionOptions}
            value={query.unitConversion}
            allowCustomValue={false}
            onChange={this.onUnitConversionChange}
          />
        </div>
      </>
    );
  }
}

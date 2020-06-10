import defaults from 'lodash/defaults';

import React, { ChangeEvent, PureComponent } from 'react';
import { InlineFormLabel, SegmentAsync } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from './DataSource';
import { defaultQuery, MyDataSourceOptions, MyQuery } from './types';

type Props = QueryEditorProps<DataSource, MyQuery, MyDataSourceOptions>;

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

    //onChange({ ...query, channel: item.value });
    query.keyword = item.value;
    query.queryText = query.service + '.' + query.keyword;
    onChange({ ...query, keyword: item.value });
    onRunQuery();
  };

  onQueryTextChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onChange, query } = this.props;
    onChange({ ...query, queryText: event.target.value });
  };

  onConstantChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onChange, query, onRunQuery } = this.props;
    onChange({ ...query, constant: parseFloat(event.target.value) });
    // executes the query
    onRunQuery();
  };

  render() {
    const datasource = this.props.datasource;
    const query = defaults(this.props.query, defaultQuery);

    return (
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
    );
  }
}

import { asArray } from '../format';
import { t } from '../i18n';
import type { ViewPreset } from '../types';
import type { SwitchFilter } from '../views/props';

export function SwitchFilters({ filter, onChange, switches, groups, parts }: {
  filter: SwitchFilter;
  onChange: (filter: SwitchFilter) => void;
  switches: readonly { id: number; name: string; group_tag: string }[];
  groups?: string[];
  parts?: ViewPreset[];
}) {
  const groupNames = new Set([...asArray(groups), ...switches.map(sw => sw.group_tag)]);
  if (filter.group !== undefined) groupNames.add(filter.group);
  return <div className="screen-filters">
    <label>{t('filters.group')}
      <select value={filter.group === undefined ? '' : `group:${filter.group}`}
        onChange={event => onChange({ ...filter, group: event.target.value ? event.target.value.slice(6) : undefined })}>
        <option value="">{t('rack.all')}</option>
        {[...groupNames].map(group => <option key={group} value={`group:${group}`}>{group || t('rack.ungrouped')}</option>)}
      </select>
    </label>
    <label>{t('rack.part')}
      <select value={filter.part} onChange={event => onChange({ ...filter, part: event.target.value })}>
        <option value="">{t('rack.all')}</option>
        {filter.part && !parts?.some(part => part.id === filter.part) && <option value={filter.part}>{filter.part}</option>}
        {asArray(parts).map(part => <option key={part.id} value={part.id}>{part.name}</option>)}
      </select>
    </label>
    <label>{t('filters.switch')}
      <select value={filter.switchId ?? ''} onChange={event => onChange({ ...filter, switchId: Number(event.target.value) || undefined })}>
        <option value="">{t('rack.all')}</option>
        {filter.switchId !== undefined && !switches.some(sw => sw.id === filter.switchId) &&
          <option value={filter.switchId}>#{filter.switchId}</option>}
        {switches.map(sw => <option key={sw.id} value={sw.id}>{sw.name}</option>)}
      </select>
    </label>
  </div>;
}

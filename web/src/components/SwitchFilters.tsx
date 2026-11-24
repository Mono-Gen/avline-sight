import { asArray } from '../format';
import { t } from '../i18n';
import type { ViewPreset } from '../types';
import type { SwitchFilter } from '../views/props';

type FilterSwitch = { id: number; name: string; group_tag: string };

// A part preset with no tags and no VLANs (such as the default "all" preset) does not narrow anything.
function meaningfulPart(parts: readonly ViewPreset[] | undefined, id: string) {
  return asArray(parts).some(part => part.id === id && (asArray(part.tags).length > 0 || asArray(part.vlans).length > 0));
}

// Returns the filter with selections that no longer exist (renamed group, deleted switch or part) cleared,
// or undefined when nothing needs to change. Owners call this so a screen never stays empty on a stale choice.
export function pruneSwitchFilter(filter: SwitchFilter, switches: readonly FilterSwitch[],
  groups: readonly string[] | undefined, parts: readonly ViewPreset[] | undefined): SwitchFilter | undefined {
  if (!switches.length) return undefined;
  const groupNames = new Set([...asArray(groups), ...switches.map(sw => sw.group_tag)]);
  const staleGroup = filter.group !== undefined && !groupNames.has(filter.group);
  const stalePart = !!filter.part && parts !== undefined && !meaningfulPart(parts, filter.part);
  const staleSwitch = filter.switchId !== undefined && !switches.some(sw => sw.id === filter.switchId);
  if (!staleGroup && !stalePart && !staleSwitch) return undefined;
  return {
    group: staleGroup ? undefined : filter.group,
    part: stalePart ? '' : filter.part,
    switchId: staleSwitch ? undefined : filter.switchId,
  };
}

export function SwitchFilters({ filter, onChange, switches, groups, parts }: {
  filter: SwitchFilter;
  onChange: (filter: SwitchFilter) => void;
  switches: readonly FilterSwitch[];
  groups?: string[];
  parts?: ViewPreset[];
}) {
  const groupNames = new Set([...asArray(groups), ...switches.map(sw => sw.group_tag)]);
  const change = (next: SwitchFilter) => onChange({ ...next, part: next.part && meaningfulPart(parts, next.part) ? next.part : '' });
  return <div className="screen-filters">
    <label>{t('filters.group')}
      <select value={filter.group === undefined || !groupNames.has(filter.group) ? '' : `group:${filter.group}`}
        onChange={event => change({ ...filter, group: event.target.value ? event.target.value.slice(6) : undefined })}>
        <option value="">{t('rack.all')}</option>
        {[...groupNames].map(group => <option key={group} value={`group:${group}`}>{group || t('rack.ungrouped')}</option>)}
      </select>
    </label>
    <label>{t('rack.part')}
      <select value={filter.part && meaningfulPart(parts, filter.part) ? filter.part : ''}
        onChange={event => change({ ...filter, part: event.target.value })}>
        <option value="">{t('rack.all')}</option>
        {asArray(parts).filter(part => meaningfulPart(parts, part.id)).map(part => <option key={part.id} value={part.id}>{part.name}</option>)}
      </select>
    </label>
    <label>{t('filters.switch')}
      <select value={filter.switchId ?? ''} onChange={event => change({ ...filter, switchId: Number(event.target.value) || undefined })}>
        <option value="">{t('rack.all')}</option>
        {switches.map(sw => <option key={sw.id} value={sw.id}>{sw.name}</option>)}
      </select>
    </label>
  </div>;
}

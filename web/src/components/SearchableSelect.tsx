import { createSignal, For, Show, onMount, onCleanup } from 'solid-js';

export interface SelectOption {
  value: string;
  label: string;
}

interface Props {
  options: SelectOption[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
}

export default function SearchableSelect(props: Props) {
  const [search, setSearch] = createSignal('');
  const [open, setOpen] = createSignal(false);
  let containerRef: HTMLDivElement | undefined;

  const filtered = () => {
    const q = search().toLowerCase();
    return q ? props.options.filter(o => o.label.toLowerCase().includes(q)) : props.options;
  };

  const selectedLabel = () => props.options.find(o => o.value === props.value)?.label;

  function handleClickOutside(e: MouseEvent): void {
    if (containerRef && !containerRef.contains(e.target as Node)) {
      setOpen(false);
      setSearch('');
    }
  }

  onMount(() => document.addEventListener('mousedown', handleClickOutside));
  onCleanup(() => document.removeEventListener('mousedown', handleClickOutside));

  if (props.disabled) {
    return (
      <div class="form-control form-control-sm bg-body-secondary text-muted">
        {selectedLabel() ?? props.placeholder ?? '—'}
      </div>
    );
  }

  return (
    <div ref={containerRef} class="position-relative">
      <div
        class="form-control form-control-sm d-flex justify-content-between align-items-center"
        style="cursor:pointer;user-select:none"
        onClick={() => setOpen(o => !o)}
      >
        <span class={selectedLabel() ? '' : 'text-muted'}>
          {selectedLabel() ?? props.placeholder ?? 'Select...'}
        </span>
        <i class={`bi bi-chevron-${open() ? 'up' : 'down'} small text-muted ms-1`}></i>
      </div>
      <Show when={open()}>
        <div
          class="position-absolute w-100 bg-body border rounded shadow-sm"
          style="z-index:1050;top:calc(100% + 2px)"
        >
          <div class="p-1 border-bottom">
            <input
              type="text"
              class="form-control form-control-sm border-0 shadow-none"
              placeholder="Search..."
              value={search()}
              onInput={e => setSearch(e.currentTarget.value)}
              onClick={e => e.stopPropagation()}
            />
          </div>
          <div style="max-height:220px;overflow-y:auto">
            <For
              each={filtered()}
              fallback={<div class="px-3 py-2 text-muted small">No options</div>}
            >
              {(opt) => (
                <div
                  class={`px-3 py-2 small ${opt.value === props.value ? 'bg-primary text-white' : ''}`}
                  style="cursor:pointer"
                  onMouseEnter={e => {
                    if (opt.value !== props.value)
                      (e.currentTarget as HTMLElement).style.backgroundColor = 'var(--bs-secondary-bg)';
                  }}
                  onMouseLeave={e => {
                    if (opt.value !== props.value)
                      (e.currentTarget as HTMLElement).style.backgroundColor = '';
                  }}
                  onClick={() => {
                    props.onChange(opt.value);
                    setOpen(false);
                    setSearch('');
                  }}
                >
                  {opt.label}
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  );
}

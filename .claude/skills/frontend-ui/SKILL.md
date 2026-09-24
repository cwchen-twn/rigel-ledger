---
name: frontend-ui
description: How to build or change rigel-ledger's SolidJS UI in its shadcn/ui-style design - the token-only styling, the owned component kit in web/src/components/ui, when to use Kobalte vs native elements, the dialog/combobox traps, money as decimal strings, i18n keys, and how to verify in a real browser. Use when adding or editing pages, components, forms, styles or translations under web/src.
---

# Frontend UI (rigel-ledger)

SolidJS + Tailwind v4 + Kobalte, in shadcn/ui's design. shadcn/ui itself is
React-only; we keep its **look and copy-in model**: the components live in this
repo (`web/src/components/ui/`), not in node_modules, and we change them freely.

## Styling rules

- **Tokens only.** Use `bg-background`, `text-muted-foreground`, `border`,
  `bg-primary`, `text-destructive`, `text-positive`... never hex colours or raw
  palette classes. The tokens live in `web/src/styles/globals.css` (`:root` and
  `.dark`), mapped by `@theme inline`. Dark mode then works for free.
- Variants go through `cva`; class merging through `cn()` (`~/lib/cn`).
- New component: copy the shape of an existing one in `components/ui/`, keep the
  header comment crediting shadcn/ui (MIT) as the style source.
- Amounts: add `tabular-nums` so digits line up.
- **No Bootstrap, no second component library, no new UI dependency** without a
  reason written in the commit body.

## Kobalte or native?

| Need | Use |
|---|---|
| Dialog, side panel | `Dialog` / `Sheet` in `components/ui/dialog.tsx` (Kobalte) |
| Searchable pick-one (accounts) | `AccountCombobox` (Kobalte Combobox) |
| Long list (currencies, commodities, time zones): more than ~15 options | `SearchSelect` in `components/ui/search-select.tsx`, options from `~/lib/options` (`commodityOptions`, `timeZoneOptions`) |
| Row action menu | `DropdownMenu` (Kobalte) |
| Notifications | `toast.success/error` (Kobalte Toast) |
| Plain select (short list), checkbox, date | **native** `Select`, `Checkbox`, `<Input type="date">` -- accessible, mobile keyboards, no JS |

## Traps already hit -- do not reintroduce

1. **Popups inside a modal must mount inside it.** Kobalte's modal Dialog marks
   everything outside its content `aria-hidden`, so a list portaled to `<body>`
   is invisible to screen readers. `Dialog`/`Sheet` provide their content element
   via `usePortalMount()`; pass `mount={mount?.()}` to any `*.Portal` that can
   open inside a dialog. (Its portal root is hidden too; only the content works.)
2. **Stop focus theft on popup mousedown.** Inside a dialog the nearest focusable
   ancestor is the dialog itself, so pressing an option blurred the input and
   closed the list before the click selected anything (keyboard still worked).
   `onMouseDown={(e) => e.preventDefault()}` on the popup content fixes it.
3. **Async `<option>`s.** Solid sets `<select value>` once; options that load
   later made the browser show the first one (a USD line showed "AED"). The
   shared `Select` re-applies `value` when its children change -- use it, not a
   bare `<select>`.
4. **Dark native controls need `color-scheme`.** Without it the browser draws a
   `<select>`'s option list (and date pickers, scrollbars) in its light default:
   white lists on a dark page. `globals.css` sets `color-scheme` on `:root`/`.dark`
   and paints `option` with the popover tokens -- keep both.
5. **Flex/grid children must be allowed to shrink.** A Kobalte root or a `Card`
   in a grid defaults to `min-width: auto`, so an input's intrinsic width or a
   long table cell pushed the page to 816px at 390px wide. `Card` and
   `SearchSelect` carry `min-w-0`; do the same for new wrappers, and check
   `document.documentElement.scrollWidth` at 390px.
6. **`-0`.** js-big-decimal negates `"0"` to `"-0"`, which prints "-NT$0.00".
   Use `neg()` from `~/lib/money`, which returns `"0"`.

## Money and dates

- Money is a **decimal string** end to end ("1234.50"). Arithmetic only through
  `~/lib/money` (js-big-decimal): `add`, `sub`, `mul`, `neg`, `round`, `sum`.
  **Never `Number()` or `parseFloat` an amount.**
- Display with `<Money amount currency signed?>`; input with `<MoneyInput>`
  (numeric keypad, right-aligned). Decimals come from the currency list.
- Stored amounts are signed (debit > 0). Show balances with `naturalAmount(amount,
  class)` from `~/stores/book` so liabilities/equity/income read positive.
- Dates are `YYYY-MM-DD` strings; display with `formatDate(iso, user.date_format)`.

## i18n

- Every visible string goes through `t('key')`. Keys live in
  `web/src/i18n/{en,zh,es}.json` with **identical key sets** (zh is Traditional
  Chinese). Add a key to all three.
- API errors: `te(err)` (-> `error.<code>`); per-field: `fieldErrors(err)` (->
  `field.<code>`) -- pass to `<Field error=...>`.
- `<Field>` ties its label to the control: `Input`, `Select`, `Textarea` and
  `MoneyInput` take the id, `aria-describedby` and `aria-invalid` from it, so every
  field has an accessible name (and Playwright's `get_by_label` works). A new custom
  control inside a Field spreads `useFieldProps()` from `~/components/ui/input`.
- Signed-out pages and the first-login wizard sit in `<AuthCard>`. `RequireUser` in
  `App.tsx` sends a user with `initialized === false` or `password_must_change` to
  `/welcome`; the API refuses them everything else with `onboarding_required`.
- Account names: `book.name(account)` (template key -> `account.template.<key>`
  unless the user renamed it); paths: `book.path(id)`.

## Data flow

- `api/client.ts` is the only place that calls `fetch`; it adds the
  `X-Rigel-Client` CSRF header and throws `ApiError`. Keep `api/types.ts` in step
  with `internal/routes/dto.go`.
- `useSession()` holds the user and `commodities()` (everything an account can hold),
  with `currencies()` = ISO money only (base, display currency, rates), plus `kind()`,
  `decimals()`; it applies language/theme live. Pick the right list for each select.
  `<Money>` prints miles and shares as units ("9,500 EVA"), not as currency;
  `useBook()` holds the current book, accounts, roles (`canEdit`, `isOwner`).
  Hide or disable write controls for viewers; the server enforces it anyway.

## Verify before committing

```bash
cd web && bunx tsc --noEmit -p tsconfig.json && bun run build:prod
```

Then look at it in a real browser (Playwright via the webapp-testing skill works):
light and dark, desktop and ~390px wide, and at least one flow through a dialog
with the mouse. Playwright notes: native `<select>` elements also have role
`combobox`/`option` -- target Kobalte with `input[role=combobox]` and
`[role=listbox] [role=option]`, and type with `type()` (not `fill()`) so the
combobox filters.

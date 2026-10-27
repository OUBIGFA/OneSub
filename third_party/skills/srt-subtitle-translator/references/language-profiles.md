# Language Profiles

[config/language_profiles.json](../config/language_profiles.json) is the executable
configuration. Do not copy its thresholds into another settings table or treat them as
universal linguistic limits.

## Fields

- `target_language`: nonempty display name.
- `counting`: `cjk` uses weighted display cost; `raw` counts displayed characters,
  including internal spaces. Both exempt English parenthetical content from load.
- `final_punctuation`: `none` reviews trailing stopping/pausing punctuation while
  preserving genuine `?`/`？`; `standard` follows ordinary target-language punctuation.
- `max_cps`: finite positive number, reading-cost units per second.
- `max_width`: finite positive number, per-line scan-cost review threshold. This is not
  a measured pixel width or an automatically enforced screen cap.
- `spacing`: optional boolean, default `false`; review CJK/Latin and CJK/digit spacing.
- `ban_exclamation`: optional boolean, default `false`; review `!` and `！`.

Quoted numbers, booleans used as numbers, NaN, infinity, and nonpositive limits are
invalid. See [reading-load.md](reading-load.md) for counting and warning interpretation.

## Resolution

Normalize codes to lowercase and replace `_` with `-`. An exact profile wins, then the
longest configured prefix: `zh-Hant-TW` uses `zh-hant`, and `en-US` uses `en`. The
Traditional-Chinese locales `zh-TW`, `zh-HK`, and `zh-MO` use `zh-hant` unless explicitly
configured. An unknown language uses the required `default` profile and is reported.

## Overrides

Use `--lang-config <project.json>` with the same schema and `--lang <code>`. Invalid
custom files fail visibly; they do not silently fall back. `--max-cps`, `--max-width`,
`--min-duration`, `--max-duration`, `--max-lines`, and `--pause-gap` permit explicit
project calibration. The CLI rejects invalid values and contradictory duration bounds.
`--strict` requires `--source`; without a source there is no timeline to lock.

An explicit platform or user guide controls the chosen profile. Do not change the
shipped Chinese 4.5 DU/s preset merely to make a dense file's warnings disappear.

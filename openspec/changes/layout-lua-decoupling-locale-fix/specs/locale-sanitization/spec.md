# Locale Sanitization Specification

## Purpose

Ensures Fyne's locale parser receives a valid UTF-8 locale by sanitizing `LANG` and `LC_ALL` environment variables before `app.New()` is called. Prevents `Error parsing user locale C` on systems with `C`, `POSIX`, or unset locales.

## Requirements

### Requirement: sanitizeLocale Function

The system SHALL provide a `sanitizeLocale()` function that inspects `LANG` and `LC_ALL` via `os.Getenv`. The function MUST be called before `app.New()` in the bootstrap sequence.

When both `LANG` and `LC_ALL` are empty, `"C"`, or `"POSIX"`, the function MUST call `os.Setenv("LANG", "en_US.UTF-8")`.

When `LANG` is already a valid non-C, non-POSIX value (e.g. `"es_AR.UTF-8"`), the function MUST NOT mutate it.

When `LC_ALL` is set to a valid non-C, non-POSIX value, `LANG` SHALL follow `LC_ALL`.

#### Scenario: LANG is C — forces en_US.UTF-8

- GIVEN `LANG` is `"C"` and `LC_ALL` is unset
- WHEN `sanitizeLocale()` is called
- THEN `LANG` SHALL be set to `"en_US.UTF-8"`

#### Scenario: LC_ALL is POSIX — forces en_US.UTF-8

- GIVEN `LC_ALL` is `"POSIX"` and `LANG` is unset
- WHEN `sanitizeLocale()` is called
- THEN `LANG` SHALL be set to `"en_US.UTF-8"`

#### Scenario: Both empty — forces en_US.UTF-8

- GIVEN `LANG` is unset and `LC_ALL` is unset
- WHEN `sanitizeLocale()` is called
- THEN `LANG` SHALL be set to `"en_US.UTF-8"`

#### Scenario: LANG already valid — no mutation

- GIVEN `LANG` is `"es_AR.UTF-8"`
- WHEN `sanitizeLocale()` is called
- THEN `LANG` SHALL remain `"es_AR.UTF-8"`

#### Scenario: LC_ALL set and valid — LANG follows

- GIVEN `LC_ALL` is `"en_US.UTF-8"` and `LANG` is unset
- WHEN `sanitizeLocale()` is called
- THEN `LANG` SHALL be set to `"en_US.UTF-8"`

### Requirement: sanitizeLocale Ordering

`sanitizeLocale()` MUST execute before any Fyne initialization (`app.New()`). Fyne reads `LANG` during app construction — sanitizing after that point has no effect.

#### Scenario: sanitizeLocale runs before app.New

- GIVEN the bootstrap sequence starts
- WHEN `app.New()` is called
- THEN `sanitizeLocale()` SHALL have already executed
- AND `LANG` SHALL contain a valid non-C, non-POSIX value

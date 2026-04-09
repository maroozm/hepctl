# hepctl

`hepctl` is a terminal app to install HEP (High Energy Physics) packages from one place.

## Quick start

```bash
go run ./cmd/hepctl
```

```bash
go run ./cmd/hepct
```

## Commands

- `install packagename`
- `help`
- `quit`

## Package checklist

| Package | Issue | Status | Ubuntu | macOS |
| ------- | ----- | ------ | ------ | ----- |
| ROOT    | -     | 🟢 supported | [ ]    | [x]   |
| PYTHIA  | -     | 🔴 not supported | [ ]    | [ ]   |
| AMPT    | [#3](https://github.com/maroozm/hepctl/issues/3) | 🔴 not supported | [ ]    | [ ]   |
| HIJING  | -     | 🔴 not supported | [ ]    | [ ]   |
| RIVET   | -     | 🔴 not supported | [ ]    | [ ]   |
| EPOS4   | -     | 🔴 not supported | [ ]    | [ ]   |

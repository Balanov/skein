# Credits and asset licenses

## Skein logo and visual design

The Skein logo and landing-page composition were created specifically for this project. No third-party illustrations, stock photographs, or generated imagery are used.

## IBM Plex

The landing page self-hosts **IBM Plex Sans** and **IBM Plex Mono**, distributed through the npm packages `@fontsource/ibm-plex-sans` and `@fontsource/ibm-plex-mono`.

IBM Plex is licensed under the **SIL Open Font License, Version 1.1**. The packages retain the upstream copyright and license notices. Exact package versions are recorded in `site/package-lock.json`.

- IBM Plex source: https://github.com/IBM/plex
- Fontsource: https://fontsource.org/
- SIL Open Font License: https://openfontlicense.org/

## Other dependencies

Skein's Go core uses only the Go standard library. Vite and Fontsource are development/build dependencies of the static landing page. Benchmark scripts optionally use Python and LangGraph; their presence is not required to build or run Skein.

External projects discussed in [docs/RESEARCH.md](docs/RESEARCH.md) were used as design context only. No source code or graphical assets were copied from them.

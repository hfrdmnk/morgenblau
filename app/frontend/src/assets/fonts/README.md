# Fonts

Self-hosted reader and interface fonts; `fonts.css` declares them. Their licenses live in `public/fonts/<family>/`, so the build serves them beside the fonts; Vite emits only imported files from here.

- `geist/`: Geist by Vercel ([vercel/geist-font](https://github.com/vercel/geist-font)), the variable `wght` upright and italic subsets published by Fontsource as `@fontsource-variable/geist` 5.3.0.
- `newsreader/`: Newsreader by Production Type ([google/fonts](https://github.com/google/fonts/tree/main/ofl/newsreader)), the variable `wght` and `opsz` ("standard") subsets published by Fontsource as `@fontsource-variable/newsreader` 5.3.0.
- `ia-writer-duo/`: iA Writer Duo by Information Architects, from `iA Writer Duo/Variable/` in [iaolo/iA-Fonts](https://github.com/iaolo/iA-Fonts).

"iA Writer" is a Reserved Font Name, so the Duo files must stay unmodified apart from WOFF2 compression ([OFL-FAQ 2.2](https://openfontlicense.org/ofl-faq/)): never subset or instance them. They were converted without the glyf transform so the glyph tables stay byte-identical:

```sh
uvx --from fonttools --with brotli python -c "from fontTools.ttLib import woff2; woff2.compress('iAWriterDuoV.ttf', 'ia-writer-duo-variable.woff2', transform_tables=())"
```

Geist and Newsreader reserve no names, so their files may be subset.

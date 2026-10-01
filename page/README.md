# golunch landing page

Static React site for the repository, deployed to GitHub Pages by
`.github/workflows/pages.yml` on every push touching this directory.

```console
$ npm install
$ npm run dev      # http://localhost:3000
$ npm run build    # emits dist/, which the workflow uploads as the Pages artifact
```

The build uses a relative base (`vite.config.ts`), so the same artifact works
under any project Pages URL.

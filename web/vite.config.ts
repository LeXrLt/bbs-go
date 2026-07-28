import path from "node:path"
import { reactRouter } from "@react-router/dev/vite"
import { defineConfig, loadEnv, type Plugin } from "vite"

function stripSpaServerRouteExports(): Plugin {
  const appRoutesDir = `${path.sep}app${path.sep}routes${path.sep}`
  const appRootFile = `${path.sep}app${path.sep}root.tsx`

  return {
    name: "bbsgo-strip-spa-server-route-exports",
    enforce: "pre",
    transform(code, id) {
      if (process.env.BBSGO_WEB_SPA !== "true") {
        return null
      }

      const isAppRoute = id.includes(appRoutesDir)
      const isAppRoot = id.split("?", 1)[0].endsWith(appRootFile)
      if (!isAppRoute && !isAppRoot) return null

      let nextCode = code
      if (isAppRoute) {
        nextCode = nextCode
          .replace(
            /^\s*export\s*\{\s*loader\s*\}\s*from\s*["'][^"']+["'];?\s*$/gm,
            ""
          )
          .replace(/\bexport\s+(async\s+function\s+loader\b)/g, "$1")
          .replace(/\bexport\s+(function\s+loader\b)/g, "$1")
          .replace(/\bexport\s+(const\s+loader\s*=)/g, "$1")
      }
      if (isAppRoot) {
        nextCode = nextCode.replace(/\bexport\s+(const\s+middleware\b)/g, "$1")
      }

      return nextCode === code ? null : { code: nextCode, map: null }
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, __dirname, "")
  const serverURL = env.BBSGO_SERVER_URL

  if (!serverURL) {
    throw new Error("BBSGO_SERVER_URL is required. Set it in web/.env.")
  }
  process.env.BBSGO_SERVER_URL = serverURL

  return {
    plugins: [stripSpaServerRouteExports(), reactRouter()],
    optimizeDeps: {
      include: ["md-editor-rt"],
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname),
        tailwindcss: path.resolve(
          __dirname,
          "node_modules/tailwindcss/index.css"
        ),
      },
    },
    server: {
      port: 3000,
      proxy: {
        "/api": serverURL,
        "/res": serverURL,
        "/sitemap.xml": serverURL,
      },
    },
  }
})

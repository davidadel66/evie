import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // `vite dev` serves the UI on 5173 and forwards the API to a running
    // `evie serve`. The Go guard already accepts a localhost:5173 Origin.
    proxy: {
      "/api": "http://127.0.0.1:6687",
    },
  },
});

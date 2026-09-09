/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        brand: {
          50: "#eef4ff",
          100: "#d9e6ff",
          500: "#3b6fe0",
          600: "#2f59c4",
          700: "#274aa3",
        },
      },
    },
  },
  plugins: [],
};

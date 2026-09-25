import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App } from "./App";
import { AuthProvider } from "./auth/AuthContext";
import { ThemeProvider } from "./contexts/ThemeContext";
import TooltipLayer from "./components/ui/Tooltip";
import "./index.css";

// The development preview draws the shared pieces with sample data.
if (import.meta.env.DEV && window.location.pathname === "/__preview") {
  void import("./dev/Preview").then(({ default: Preview }) => {
    createRoot(document.getElementById("root")!).render(
      <StrictMode>
        <ThemeProvider>
          <Preview />
          <TooltipLayer />
        </ThemeProvider>
      </StrictMode>,
    );
  });
} else
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <BrowserRouter>
        <AuthProvider>
          <App />
          <TooltipLayer />
        </AuthProvider>
      </BrowserRouter>
    </ThemeProvider>
  </StrictMode>,
);

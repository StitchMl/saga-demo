import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { FlowProvider } from "./context/FlowContext";
import {AuthProvider} from "./context/AuthContext";

const root = createRoot(document.getElementById("root"));
root.render(
    <React.StrictMode>
        <AuthProvider>
            <FlowProvider>
                <App />
            </FlowProvider>
        </AuthProvider>
    </React.StrictMode>
);
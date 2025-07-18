import { BrowserRouter, Routes, Route } from "react-router-dom";
import { CssBaseline, ThemeProvider } from "@mui/material";
import theme from "./theme";

import { AuthProvider } from "./context/AuthContext";
import NavBar from "./components/NavBar";
import Home from "./pages/Home";
import Catalog from "./pages/Catalog";
import CreateOrder from "./pages/CreateOrder";
import CreateItem from "./pages/CreateItem";
import LoginRegister from "./pages/LoginRegister";
import OrderStatus from "./pages/OrderStatus";

export default function App() {
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <AuthProvider>
                <BrowserRouter>
                    <NavBar />
                    <Routes>
                        <Route path="/" element={<Home />} />
                        <Route path="/catalog" element={<Catalog />} />
                        <Route path="/create" element={<CreateOrder />} />
                        <Route path="/create-items" element={<CreateItem />} />
                        <Route path="/status" element={<OrderStatus />} />
                        <Route path="/login" element={<LoginRegister />} />
                    </Routes>
                </BrowserRouter>
            </AuthProvider>
        </ThemeProvider>
    );
}

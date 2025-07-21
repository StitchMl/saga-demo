import { BrowserRouter, Routes, Route, useNavigate } from "react-router-dom";
import { CssBaseline, ThemeProvider } from "@mui/material";
import theme from "./theme";

import { AuthProvider, useAuth } from "./context/AuthContext";
import { FlowProvider } from "./context/FlowContext";
import NavBar from "./components/NavBar";
import Home from "./pages/Home";
import Catalog from "./pages/Catalog";
import CreateOrder from "./pages/CreateOrder";
import LoginRegister from "./pages/LoginRegister";
import OrderStatus from "./pages/OrderStatus";
import OrderDetail from "./pages/OrderDetail";
import { useEffect } from "react";

// Component to handle redirection after logout
const AuthRedirect = () => {
    const { customerId } = useAuth();
    const navigate = useNavigate();

    useEffect(() => {
        // Se customerId diventa null/vuoto e non siamo già sulla pagina di login, reindirizza.
        if (!customerId && window.location.pathname !== '/login') {
            navigate('/login');
        }
    }, [customerId, navigate]);

    return null; // This component doesn't render anything
};

export default function App() {
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter>
                <AuthProvider>
                    <FlowProvider>
                        <NavBar />
                        <AuthRedirect />
                        <Routes>
                            <Route path="/" element={<Home />} />
                            <Route path="/catalog" element={<Catalog />} />
                            <Route path="/create" element={<CreateOrder />} />
                            <Route path="/status" element={<OrderStatus />} />
                            <Route path="/orders/:orderId" element={<OrderDetail />} />
                            <Route path="/login" element={<LoginRegister />} />
                        </Routes>
                    </FlowProvider>
                </AuthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

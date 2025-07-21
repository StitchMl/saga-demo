import {
    Box,
    TextField,
    Button,
    Tabs,
    Tab,
    Snackbar,
    Alert
} from "@mui/material";
import { useState } from "react";
import { login, register } from "../api";
import { useAuth } from "../context/AuthContext";
import { useFlow } from "../context/FlowContext";
import { useNavigate } from "react-router-dom";

export default function LoginRegister() {
    const [tab, setTab] = useState(0);
    const [form, setForm] = useState({});
    const [snack, setSnack] = useState({ open: false, msg: "", severity: "info" });
    const { login: authLogin } = useAuth();
    const { flow } = useFlow();
    const navigate = useNavigate();

    const handleChange = (e) =>
        setForm({ ...form, [e.target.name]: e.target.value });

    const handleSubmit = async (e) => {
        e.preventDefault();
        try {
            if (tab === 0) {
                const { data } = await login(form.username, form.password, flow);
                authLogin(data.customer_id);
            } else {
                const { data } = await register(
                    form.username,
                    form.password,
                    form.name,
                    form.email,
                    flow
                );
                authLogin(data.customer_id);
            }
            navigate("/");
        } catch (err) {
            setSnack({
                open: true,
                msg: err.response?.data || err.message,
                severity: "error"
            });
        }
    };

    return (
        <Box sx={{ maxWidth: 400, mx: "auto", mt: 4 }}>
            <Tabs value={tab} onChange={(_, v) => setTab(v)} centered>
                <Tab label="Login" />
                <Tab label="Registrati" />
            </Tabs>
            <Box
                component="form"
                onSubmit={handleSubmit}
                sx={{ display: "flex", flexDirection: "column", gap: 2, mt: 2 }}
            >
                {tab === 1 && (
                    <>
                        <TextField
                            label="Nome"
                            name="name"
                            required
                            onChange={handleChange}
                        />
                        <TextField
                            label="Email"
                            name="email"
                            type="email"
                            required
                            onChange={handleChange}
                        />
                    </>
                )}
                <TextField
                    label="Username"
                    name="username"
                    required
                    onChange={handleChange}
                />
                <TextField
                    label="Password"
                    name="password"
                    type="password"
                    required
                    onChange={handleChange}
                />
                <Button type="submit" variant="contained">
                    {tab === 0 ? "Accedi" : "Registrati"}
                </Button>
            </Box>

            <Snackbar
                open={snack.open}
                autoHideDuration={4000}
                onClose={() => setSnack({ ...snack, open: false })}
            >
                <Alert severity={snack.severity}>{snack.msg}</Alert>
            </Snackbar>
        </Box>
    );
}

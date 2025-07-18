import { useLocation } from "react-router-dom";
import {
    Box,
    Button,
    TextField,
    Typography,
    Snackbar,
    Alert
} from "@mui/material";
import { useState } from "react";
import { useAuth } from "../context/AuthContext";
import { useOrderApi } from "../api";

export default function CreateOrder() {
    const { state } = useLocation();
    const presetProduct = state?.product;

    const { customerId } = useAuth();
    const { createOrder, flow } = useOrderApi();

    const [productId, setProductId] = useState(presetProduct?.id || "");
    const [quantity, setQuantity] = useState(1);
    const [snack, setSnack] = useState({ open: false, msg: "", severity: "info" });

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!customerId)
            return setSnack({
                open: true,
                msg: "Devi autenticarti",
                severity: "error"
            });

        try {
            const order = {
                customer_id: customerId,
                items: [{ product_id: productId, quantity: Number(quantity) }]
            };
            const { data } = await createOrder(order);

            setSnack({
                open: true,
                msg: `Ordine (${flow}) inviato, id: ${data.order_id || "???"}`,
                severity: "success"
            });
        } catch (err) {
            setSnack({
                open: true,
                msg: err.response?.data || err.message,
                severity: "error"
            });
        }
    };

    return (
        <Box
            component="form"
            onSubmit={handleSubmit}
            sx={{
                maxWidth: 400,
                mx: "auto",
                mt: 4,
                display: "flex",
                flexDirection: "column",
                gap: 2
            }}
        >
            <Typography variant="h5">Nuovo Ordine ({flow})</Typography>

            <TextField
                label="Product ID"
                required
                value={productId}
                onChange={(e) => setProductId(e.target.value)}
            />

            <TextField
                label="Quantity"
                type="number"
                inputProps={{ min: 1 }}
                required
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
            />

            <Button type="submit" variant="contained">
                Invia
            </Button>

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
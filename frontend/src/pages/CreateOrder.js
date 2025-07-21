import { useLocation } from "react-router-dom";
import {
    Box,
    Button,
    TextField,
    Typography,
    Snackbar,
    Alert,
    CircularProgress,
    Card,
    CardContent,
    CardActions,
    Stack,
    IconButton
} from "@mui/material";
import { useState, useEffect } from "react";
import { Add, Delete } from "@mui/icons-material";
import { useAuth } from "../context/AuthContext";
import { useOrderApi } from "../api";

export default function CreateOrder() {
    const { state } = useLocation();
    const presetProduct = state?.product;

    const { customerId } = useAuth();
    const { createOrder, fetchOrder, flow } = useOrderApi();

    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(false);
    const [snack, setSnack] = useState({ open: false, msg: "", severity: "info" });
    const [error, setError] = useState("");

    useEffect(() => {
        if (presetProduct) {
            setItems([{ id: Date.now(), productId: presetProduct.id, qty: 1 }]);
        } else {
            setItems([{ id: Date.now(), productId: "", qty: 1 }]);
        }
    }, [presetProduct]);

    const addRow = () =>
        setItems((prev) => [
            ...prev,
            { id: Date.now() + Math.random(), productId: "", qty: 1 }
        ]);

    const removeRow = (id) =>
        setItems((prev) => prev.filter((it) => it.id !== id));

    const handleChange = (idx, field, val) =>
        setItems((prev) =>
            prev.map((it, i) => (i === idx ? { ...it, [field]: val } : it))
        );

    const pollOrderStatus = (orderId) => {
        const interval = setInterval(async () => {
            try {
                const { data: updatedOrder } = await fetchOrder(orderId);
                if (updatedOrder.status !== 'pending') {
                    clearInterval(interval);
                    setLoading(false);
                    setItems([{ id: Date.now(), productId: "", qty: 1 }]);
                    if (updatedOrder.status === 'approved') {
                        setSnack({
                            open: true,
                            msg: `Order (${flow}) approved successfully! ID: ${orderId}`,
                            severity: "success"
                        });
                    } else { // rejected
                        const finalMessage = `Rejected Order: ${updatedOrder.reason || 'Unknown reason'}`;
                        setError(finalMessage);
                        setSnack({ open: true, msg: finalMessage, severity: "error" });
                    }
                }
            } catch (pollErr) {
                clearInterval(interval);
                setLoading(false);
                const errorMsg = "Unable to verify final order status.";
                setError(errorMsg);
                setSnack({ open: true, msg: errorMsg, severity: "error" });
            }
        }, 2000);

        // Security timeout to interrupt polling
        setTimeout(() => {
            clearInterval(interval);
        }, 30000);
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!customerId) {
            return setError("You must authenticate yourself before creating an order.");
        }
        if (items.some(it => !it.productId || !it.qty || it.qty < 1)) {
            return setError("Make sure that all items have a valid product ID and quantity.");
        }

        setError("");
        setLoading(true);
        try {
            const order = {
                items: items.map(({ productId, qty }) => ({
                    product_id: productId,
                    quantity: Number(qty)
                }))
            };
            const { data } = await createOrder(order);

            if (flow === 'orchestrated') {
                // The orchestrated flow is synchronous and returns 200 OK if successful
                setLoading(false);
                setItems([{ id: Date.now(), productId: "", qty: 1 }]);
                setSnack({
                    open: true,
                    msg: `Order (${flow}) approved successfully! ID: ${data.order_id}`,
                    severity: "success"
                });
            } else {
                // The choreographed flow is asynchronous and returns 202 Accepted
                setSnack({
                    open: true,
                    msg: `Order sent (ID: ${data.order_id}). Awaiting final confirmation...`,
                    severity: "info"
                });
                pollOrderStatus(data.order_id);
            }
        } catch (err) {
            const errorMsg = err.response?.data?.reason || err.response?.data?.message || "Unknown error";
            const finalMessage = `Order Rejected: ${errorMsg}`;
            setError(finalMessage);
            setSnack({ open: true, msg: finalMessage, severity: "error" });
            setLoading(false);
        }
    };

    return (
        <Box sx={{ mt: 4, display: "flex", justifyContent: "center" }}>
            <Card sx={{ width: 600, p: 2 }}>
                <form onSubmit={handleSubmit}>
                    <CardContent>
                        <Typography variant="h5" gutterBottom>
                            Create New Order ({flow})
                        </Typography>

                        {items.map((it, idx) => (
                            <Stack
                                key={it.id}
                                direction="row"
                                spacing={1}
                                sx={{ mb: 1, alignItems: "center" }}
                            >
                                <TextField
                                    label="Product ID"
                                    value={it.productId}
                                    onChange={(e) =>
                                        handleChange(idx, "productId", e.target.value)
                                    }
                                    fullWidth
                                    required
                                />
                                <TextField
                                    label="Quantity"
                                    type="number"
                                    inputProps={{ min: 1 }}
                                    value={it.qty}
                                    onChange={(e) => handleChange(idx, "qty", e.target.value)}
                                    sx={{ width: 120 }}
                                    required
                                />
                                <IconButton
                                    aria-label="Remove Article"
                                    onClick={() => removeRow(it.id)}
                                    disabled={items.length === 1}
                                >
                                    <Delete />
                                </IconButton>
                            </Stack>
                        ))}

                        {error && (
                            <Alert severity="error" sx={{ mt: 2 }}>
                                {error}
                            </Alert>
                        )}
                    </CardContent>

                    <CardActions sx={{ justifyContent: "space-between", p: 2 }}>
                        <Button
                            startIcon={<Add />}
                            onClick={addRow}
                            variant="outlined"
                            type="button"
                        >
                            Aggiungi Articolo
                        </Button>

                        <Button
                            variant="contained"
                            type="submit"
                            disabled={loading || items.length === 0}
                            startIcon={loading ? <CircularProgress size={20} color="inherit" /> : null}
                        >
                            Invia Ordine
                        </Button>
                    </CardActions>
                </form>
            </Card>
            <Snackbar
                open={snack.open}
                autoHideDuration={6000}
                onClose={() => setSnack({ ...snack, open: false })}
                anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
            >
                <Alert severity={snack.severity} sx={{ width: "100%" }} onClose={() => setSnack({ ...snack, open: false })}>
                    {snack.msg}
                </Alert>
            </Snackbar>
        </Box>
    );
}
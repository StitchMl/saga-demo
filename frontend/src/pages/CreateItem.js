import { useState } from "react";
import {
    Box,
    Card,
    CardContent,
    CardActions,
    Typography,
    IconButton,
    TextField,
    Button,
    Stack,
    Alert
} from "@mui/material";
import { Add, Delete } from "@mui/icons-material";
import { useOrderApi } from "../api";
import { useAuth } from "../context/AuthContext";

export default function CreateItem() {
    const { customerId } = useAuth();
    const { createOrder, flow } = useOrderApi();

    const [items, setItems] = useState([
        { id: Date.now(), productId: "", qty: 1 }
    ]);
    const [error, setError] = useState("");

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

    const handleSubmit = async (e) => {
        e.preventDefault();
        if (!customerId)
            return setError("You must authenticate yourself before creating an order.");

        setError("");
        try {
            const order = {
                customer_id: customerId,
                items: items.map(({ productId, qty }) => ({
                    product_id: productId,
                    quantity: Number(qty)
                }))
            };
            await createOrder(order);
            setItems([{ id: Date.now(), productId: "", qty: 1 }]);
            alert(`Ordine (${flow}) creato!`);
        } catch (err) {
            setError(err.response?.data || err.message);
        }
    };

    return (
        <Box sx={{ mt: 4, display: "flex", justifyContent: "center" }}>
            <Card sx={{ width: 500, p: 2 }}>
                <form onSubmit={handleSubmit}>
                    <CardContent>
                        <Typography variant="h5" gutterBottom>
                            Ordine multi‑item ({flow})
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
                                    label="Qty"
                                    type="number"
                                    inputProps={{ min: 1 }}
                                    value={it.qty}
                                    onChange={(e) => handleChange(idx, "qty", e.target.value)}
                                    sx={{ width: 90 }}
                                    required
                                />
                                <IconButton
                                    aria-label="remove line"
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

                    <CardActions sx={{ justifyContent: "space-between" }}>
                        <Button
                            startIcon={<Add />}
                            onClick={addRow}
                            variant="outlined"
                            type="button"
                        >
                            Add row
                        </Button>

                        <Button variant="contained" type="submit">
                            Send Order
                        </Button>
                    </CardActions>
                </form>
            </Card>
        </Box>
    );
}
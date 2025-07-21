import { useEffect, useState } from "react";
import { useParams, Link as RouterLink } from "react-router-dom";
import {
    Box,
    CircularProgress,
    Card,
    CardContent,
    Typography,
    Chip,
    Alert,
    Button,
    Grid
} from "@mui/material";
import { useOrderApi } from "../api";

/**
 * @param {string} s
 * @returns {"success" | "error" | "warning"}
 */
const statusColor = (s) => {
    if (s === "approved" || s === "completed") return "success";
    if (s === "rejected" || s === "failed") return "error";
    return "warning";
};

export default function OrderDetail() {
    const { orderId } = useParams();
    const { fetchOrder, flow } = useOrderApi();
    const [order, setOrder] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState("");

    useEffect(() => {
        const getOrder = async () => {
            setLoading(true);
            setError("");
            try {
                const { data } = await fetchOrder(orderId);
                setOrder(data);
            } catch (e) {
                console.error(e);
                setError("Impossibile recuperare i dettagli dell'ordine.");
            } finally {
                setLoading(false);
            }
        };
        void getOrder();
    }, [orderId, fetchOrder]);

    if (loading) {
        return (
            <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
                <CircularProgress />
            </Box>
        );
    }

    if (error) {
        return <Alert severity="error" sx={{ m: 4 }}>{error}</Alert>;
    }

    if (!order) {
        return <Alert severity="info" sx={{ m: 4 }}>Dettagli ordine non trovati.</Alert>;
    }

    return (
        <Box sx={{ p: 4 }}>
            <Button component={RouterLink} to="/status" sx={{ mb: 2 }}>
                &larr; Torna a Stato Ordini
            </Button>
            <Card>
                <CardContent>
                    <Typography variant="h5" gutterBottom>
                        Dettaglio Ordine
                    </Typography>
                    <Grid container spacing={2}>
                        <Grid item xs={12} sm={6}>
                            <Typography variant="body1"><strong>ID Ordine:</strong> {order.order_id}</Typography>
                        </Grid>
                        <Grid item xs={12} sm={6}>
                            <Typography variant="body1"><strong>Flusso SAGA:</strong> {order.saga_type || flow}</Typography>
                        </Grid>
                        <Grid item xs={12} sm={6}>
                            <Typography variant="body1"><strong>Stato:</strong> <Chip label={order.status} color={statusColor(order.status)} size="small" /></Typography>
                        </Grid>
                        <Grid item xs={12} sm={6}>
                            <Typography variant="body1"><strong>Importo Totale:</strong> € {typeof order.total === "number" ? order.total.toFixed(2) : "-"}</Typography>
                        </Grid>
                        {order.reason && (
                            <Grid item xs={12}>
                                <Typography variant="body1" sx={{ color: `${statusColor(order.status)}.dark` }}>
                                    <strong>Motivo:</strong> {order.reason}
                                </Typography>
                            </Grid>
                        )}
                    </Grid>
                </CardContent>
            </Card>
        </Box>
    );
}

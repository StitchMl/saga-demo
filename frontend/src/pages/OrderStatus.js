import { useEffect, useState, useCallback, useRef } from "react";
import {
    Box,
    CircularProgress,
    Table,
    TableHead,
    TableRow,
    TableCell,
    TableBody,
    Chip,
    Toolbar,
    Typography,
    Button,
    Stack
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import { useOrderApi } from "../api";
import { useAuth } from "../context/AuthContext";

export default function OrderStatus() {
    const { customerId } = useAuth();
    const { fetchAllOrders, flow } = useOrderApi();

    const [orders, setOrders] = useState([]);
    const [initialLoading, setInitialLoading] = useState(true);
    const [refreshing, setRefreshing] = useState(false);
    const [error, setError] = useState("");
    const [lastUpdate, setLastUpdate] = useState(null);

    const navigate = useNavigate();
    const intervalRef = useRef(null);

    const statusColor = (s) => {
        if (s === "completed") return "success";
        if (s === "rejected") return "error";
        return "warning";
    };

    // Shallow diff per evitare setState inutili (quindi lampeggi)
    const shouldUpdate = (prev, next) => {
        if (prev.length !== next.length) return true;
        // confronto semplice: concat di id+status
        const sig = (arr) => arr.map((o) => o.order_id + ":" + o.status).join("|");
        return sig(prev) !== sig(next);
    };

    const fetchOrders = useCallback(
        async (isInitial = false) => {
            if (!customerId) return;
            if (isInitial) {
                setInitialLoading(true);
            } else {
                setRefreshing(true);
            }
            setError("");
            try {
                const { data } = await fetchAllOrders(customerId);
                const list = Array.isArray(data) ? data : [];
                setLastUpdate(new Date());
                setOrders((prev) => (shouldUpdate(prev, list) ? list : prev));
            } catch (e) {
                console.error(e);
                setError("Unable to retrieve orders.");
            } finally {
                setInitialLoading(false);
                setRefreshing(false);
            }
        },
        [customerId, fetchAllOrders]
    );

    // Primo load + set polling
    useEffect(() => {
        if (!customerId) return;
        void fetchOrders(true);
        if (intervalRef.current) clearInterval(intervalRef.current);
        intervalRef.current = setInterval(() => fetchOrders(false), 5000);
        return () => {
            if (intervalRef.current) clearInterval(intervalRef.current);
        };
    }, [customerId, flow, fetchOrders]);

    if (!customerId)
        return <Box sx={{ p: 4 }}>You must be authenticated to see the orders.</Box>;

    if (initialLoading)
        return (
            <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
                <CircularProgress />
            </Box>
        );

    if (error)
        return (
            <Box sx={{ p: 4, color: "error.main" }}>
                {error}
                <Box sx={{ mt: 2 }}>
                    <Button variant="outlined" onClick={() => fetchOrders(true)}>
                        Riprova
                    </Button>
                </Box>
            </Box>
        );

    return (
        <Box sx={{ p: 2 }}>
            <Toolbar disableGutters sx={{ mb: 2, justifyContent: "space-between" }}>
                <Typography variant="h6" component="div">
                    Ordini ({flow})
                </Typography>
                <Stack direction="row" spacing={2} alignItems="center">
                    <Typography variant="body2" color="text.secondary">
                        {(() => {
                            if (!lastUpdate) return "";
                            let text = `Last update: ${lastUpdate.toLocaleTimeString()}`;
                            if (refreshing) text += " (update...)";
                            return text;
                        })()}
                    </Typography>
                    <Button
                        size="small"
                        variant="outlined"
                        disabled={refreshing}
                        onClick={() => fetchOrders(false)}
                    >
                        Update
                    </Button>
                </Stack>
            </Toolbar>

            <Table size="small">
                <TableHead>
                    <TableRow>
                        <TableCell>ID</TableCell>
                        <TableCell>Flow</TableCell>
                        <TableCell>Status</TableCell>
                        <TableCell align="right">€ Totale</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {orders.length ? (
                        orders.map((o) => (
                            <TableRow
                                key={o.order_id}
                                hover
                                sx={{ cursor: "pointer" }}
                                onClick={() => navigate(`/orders/${o.order_id}`)}
                            >
                                <TableCell>{o.order_id}</TableCell>
                                <TableCell>{flow}</TableCell>
                                <TableCell>
                                    <Chip
                                        label={o.status}
                                        color={statusColor(o.status)}
                                        size="small"
                                    />
                                </TableCell>
                                <TableCell align="right">
                                    {typeof o.total === "number" ? o.total.toFixed(2) : "-"}
                                </TableCell>
                            </TableRow>
                        ))
                    ) : (
                        <TableRow>
                            <TableCell colSpan={4} align="center">
                                No orders found.
                            </TableCell>
                        </TableRow>
                    )}
                </TableBody>
            </Table>
        </Box>
    );
}
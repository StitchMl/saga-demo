import { useEffect, useState } from "react";
import {
    Box,
    Card,
    CardContent,
    CardActions,
    Button,
    Typography,
    CircularProgress,
    Grid
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import { useOrderApi } from "../api";

export default function Catalog() {
    const { fetchCatalog, flow } = useOrderApi();
    const [products, setProducts] = useState([]);
    const [loading, setLoading] = useState(true);
    const navigate = useNavigate();

    /* fetch singolo; useEffect dipende SOLO da flow */
    const fetchData = () => {
        setLoading(true);
        fetchCatalog()
            .then(({ data }) =>
                data.map((p) => ({
                    ...p,
                    price: typeof p.price === "number" ? p.price : 0,
                    available: typeof p.available === "number" ? p.available : 0
                }))
            )
            .then(setProducts)
            .catch(console.error)
            .finally(() => setLoading(false));
    };

    useEffect(fetchData, [flow]);

    if (loading)
        return (
            <Box sx={{ display: "flex", justifyContent: "center", mt: 4 }}>
                <CircularProgress />
            </Box>
        );

    return (
        <Grid container spacing={2} sx={{ p: 2 }}>
            {products.map((p) => (
                <Grid item xs={12} sm={6} md={4} key={p.id}>
                    <Card>
                        <CardContent>
                            <Typography variant="h6">{p.name}</Typography>
                            <Typography variant="body2" color="text.secondary">
                                {p.description || "Nessuna descrizione"}
                            </Typography>
                            <Typography sx={{ mt: 1 }} variant="subtitle1">
                                € {p.price.toFixed(2)}
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                Disponibili: <strong>{p.available}</strong>
                            </Typography>
                        </CardContent>
                        <CardActions>
                            <Button
                                size="small"
                                disabled={p.available === 0}
                                onClick={() => navigate("/create", { state: { product: p } })}
                            >
                                Ordina
                            </Button>
                            {/* refresh manuale, senza innescare loading spinner */}
                            <Button size="small" onClick={fetchData}>
                                Aggiorna
                            </Button>
                        </CardActions>
                    </Card>
                </Grid>
            ))}
        </Grid>
    );
}
import { useEffect, useState, useCallback } from "react";
import {
    Box,
    Card,
    CardContent,
    CardActions,
    Button,
    Typography,
    CircularProgress,
    Grid,
    CardMedia
} from "@mui/material";
import { useNavigate } from "react-router-dom";
import { useOrderApi } from "../api";

export default function Catalog() {
    const { fetchCatalog } = useOrderApi();
    const [products, setProducts] = useState([]);
    const [loading, setLoading] = useState(true);
    const navigate = useNavigate();

    const fetchData = useCallback(async () => {
        setLoading(true);
        try {
            const { data } = await fetchCatalog();
            if (Array.isArray(data)) {
                const formattedProducts = data.map((p) => ({
                    ...p,
                    price: typeof p.price === "number" ? p.price : 0,
                    available: typeof p.available === "number" ? p.available : 0,
                }));
                setProducts(formattedProducts);
            } else {
                console.error("The API response is not an array:", data);
                setProducts([]);
            }
        } catch (error) {
            console.error("Error in retrieving catalogue:", error);
            setProducts([]);
        } finally {
            setLoading(false);
        }
    }, [fetchCatalog]);

    useEffect(() => {
        (async () => {
            await fetchData();
        })();
    }, [fetchData]);

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
                    <Card sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
                        <CardMedia
                            component="img"
                            height="140"
                            image={p.image_url || `https://via.placeholder.com/300x150.png/007bff/FFFFFF?text=${p.name}`}
                            alt={p.name}
                            sx={{ objectFit: 'cover' }}
                        />
                        <CardContent sx={{ flexGrow: 1 }}>
                            <Typography variant="h6" component="div">
                                {p.name}
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                {p.description || "No description available."}
                            </Typography>
                            <Box
                                sx={{
                                    display: "flex",
                                    justifyContent: "space-between",
                                    alignItems: "baseline",
                                    mt: 2
                                }}
                            >
                                <Typography variant="h5" color="primary">
                                    €{p.price.toFixed(2)}
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Disponibili: <strong>{p.available}</strong>
                                </Typography>
                            </Box>
                        </CardContent>
                        <CardActions>
                            <Button
                                size="small"
                                disabled={p.available === 0}
                                onClick={() => navigate("/create", { state: { product: p } })}
                            >
                                Ordina
                            </Button>
                            {/* a manual refresh, without triggering the loading spinner */}
                            <Button size="small" onClick={fetchData}>
                                Aggiorna disponibilità
                            </Button>
                        </CardActions>
                    </Card>
                </Grid>
            ))}
        </Grid>
    );
}
import { Box, Typography } from "@mui/material";

export default function Home() {
    return (
        <Box sx={{ p: 4 }}>
            <Typography variant="h4" gutterBottom>
                Welcome to the Saga demo
            </Typography>
            <Typography variant="body1">
                Use the navigation bar to browse the catalog, create orders
                (choreographed or orchestrated mode) and see the status of orders.
            </Typography>
        </Box>
    );
}
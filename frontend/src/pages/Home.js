import { Box, Typography } from "@mui/material";

export default function Home() {
    return (
        <Box sx={{ p: 4 }}>
            <Typography variant="h4" gutterBottom>
                Welcome to the Pattern SAGA Demo
            </Typography>
            <Typography variant="body1">
                This application demonstrates the implementation of the SAGA pattern
                pattern in a microservice architecture.
            </Typography>
            <Typography variant="body1" sx={{ mt: 2 }}>
                Use the navigation bar to:
                <ul>
                    <li>
                        Choose SAGA mode: <b>Choreographed</b> or{" "}
                        <b>Orchestrated</b>.
                    </li>
                    <li>Consult the <b>Catalog</b> products.</li>
                    <li>Create a <b>New Order</b>.</li>
                    <li>
                        Check the <b>Status of existing Orders</b>.
                    </li>
                </ul>
            </Typography>
        </Box>
    );
}
import React from "react";
import PropTypes from "prop-types";
import { Box, Typography } from "@mui/material";

export const TransactionDetails = ({ data }) => (
    <Box sx={{ p: 2, backgroundColor: "grey.100", borderRadius: 2, mt: 2 }}>
        <Typography variant="h6" gutterBottom>
            Transaction Details
        </Typography>
        <pre style={{ whiteSpace: "pre-wrap", wordBreak: "break-word", margin: 0 }}>
      {JSON.stringify(data, null, 2)}
    </pre>
    </Box>
);

TransactionDetails.propTypes = {
    data: PropTypes.any.isRequired
};

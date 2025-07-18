import React from "react";
import PropTypes from "prop-types";
import { Box, Typography } from "@mui/material";

const JsonView = ({ data }) => (
    <Box sx={{ p: 2 }}>
        <Typography variant="h6" gutterBottom>
            Dettagli
        </Typography>
        <pre style={{ whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
      {JSON.stringify(data, null, 2)}
    </pre>
    </Box>
);

JsonView.propTypes = {
    data: PropTypes.any.isRequired
};

export default JsonView;
